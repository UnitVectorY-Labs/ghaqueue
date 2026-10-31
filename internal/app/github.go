package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type client struct {
	http  *http.Client
	token string
	base  string
}

func newClient(ctx context.Context) *client {
	token := os.Getenv("GHAQUEUE_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		cmd := exec.CommandContext(ctx, "gh", "auth", "token", "--hostname", "github.com")
		if b, e := cmd.Output(); e == nil {
			token = strings.TrimSpace(string(b))
		}
	}
	return &client{http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, token: token, base: "https://api.github.com"}
}
func (c *client) get(ctx context.Context, path string, v any) error {
	req, e := http.NewRequestWithContext(ctx, "GET", c.base+path, nil)
	if e != nil {
		return e
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", "ghaqueue")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, e := c.http.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("GitHub %s: HTTP %d (remaining=%s, reset=%s, retry-after=%s)", strings.Split(path, "?")[0], res.StatusCode, res.Header.Get("X-RateLimit-Remaining"), res.Header.Get("X-RateLimit-Reset"), res.Header.Get("Retry-After"))
	}
	return json.NewDecoder(io.LimitReader(res.Body, 32<<20)).Decode(v)
}
func (c *client) repos(ctx context.Context, cfg Config) ([]string, error) {
	if cfg.Repo != "" {
		return []string{cfg.Repo}, nil
	}
	repos := []string{}
	for page := 1; ; page++ {
		var a []struct {
			FullName string `json:"full_name"`
			Archived bool   `json:"archived"`
			Disabled bool   `json:"disabled"`
		}
		if e := c.get(ctx, fmt.Sprintf("/orgs/%s/repos?per_page=100&page=%d&type=all", cfg.Organization, page), &a); e != nil {
			return nil, e
		}
		for _, r := range a {
			if !r.Archived && !r.Disabled {
				repos = append(repos, r.FullName)
			}
		}
		if len(a) < 100 {
			break
		}
	}
	sort.Strings(repos)
	return repos, nil
}
func (c *client) runs(ctx context.Context, repo, status string) ([]RunInfo, error) {
	runs := []RunInfo{}
	for page := 1; page <= 10; page++ {
		var data struct {
			Total int       `json:"total_count"`
			Runs  []RunInfo `json:"workflow_runs"`
		}
		q := url.Values{"status": {status}, "per_page": {"100"}, "page": {strconv.Itoa(page)}}
		if e := c.get(ctx, "/repos/"+repo+"/actions/runs?"+q.Encode(), &data); e != nil {
			return runs, e
		}
		for _, r := range data.Runs {
			r.Repository = repo
			runs = append(runs, r)
		}
		if len(data.Runs) < 100 || len(runs) >= data.Total {
			return runs, nil
		}
	}
	return runs, fmt.Errorf("%s status=%s: reached GitHub's 1,000 filtered-run limit; results may be incomplete", repo, status)
}
func (c *client) jobs(ctx context.Context, repo string, r RunInfo) ([]Job, error) {
	jobs := []Job{}
	// Pin the attempt so reruns do not silently mix attempts during collection.
	for page := 1; ; page++ {
		var data struct {
			Jobs []Job `json:"jobs"`
		}
		p := fmt.Sprintf("/repos/%s/actions/runs/%d/attempts/%d/jobs?per_page=100&page=%d", repo, r.ID, r.Attempt, page)
		if e := c.get(ctx, p, &data); e != nil {
			return jobs, e
		}
		jobs = append(jobs, data.Jobs...)
		if len(data.Jobs) < 100 {
			return jobs, nil
		}
	}
}
func (c *client) runners(ctx context.Context, scope string) ([]Runner, error) {
	all := []Runner{}
	for page := 1; ; page++ {
		var data struct {
			Runners []Runner `json:"runners"`
		}
		if e := c.get(ctx, fmt.Sprintf("/%s/actions/runners?per_page=100&page=%d", scope, page), &data); e != nil {
			return all, e
		}
		for _, r := range data.Runners {
			r.Scope = scope
			all = append(all, r)
		}
		if len(data.Runners) < 100 {
			return all, nil
		}
	}
}
func collect(ctx context.Context, c *client, cfg Config, command string) (Snapshot, error) {
	s := Snapshot{StartedAt: time.Now().UTC(), Runs: []RunInfo{}, Warnings: []string{}}
	repos, e := c.repos(ctx, cfg)
	if e != nil {
		return s, e
	}
	s.Repositories = len(repos)
	var mu sync.Mutex
	tasks := make(chan string)
	var wg sync.WaitGroup
	since, _ := time.ParseDuration(cfg.Since)
	cutoff := s.StartedAt.Add(-since)
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for repo := range tasks {
				local := []RunInfo{}
				warnings := []string{}
				statuses := []string{"queued", "in_progress", "waiting", "pending", "requested"}
				if command == "failures" {
					statuses = []string{"failure", "timed_out", "action_required"}
				}
				seen := map[int64]bool{}
				for _, status := range statuses {
					runs, err := c.runs(ctx, repo, status)
					if err != nil {
						warnings = append(warnings, err.Error())
					}
					for _, r := range runs {
						if seen[r.ID] || !matchesWorkflow(r, cfg.Workflow) {
							continue
						}
						seen[r.ID] = true
						if command == "failures" {
							if r.UpdatedAt.Before(cutoff) {
								continue
							}
							if cfg.Status != "" && r.Status != cfg.Status {
								continue
							}
						} else if !active(r.Status) {
							continue
						}
						if command != "failures" || cfg.Label != "" || cfg.RunnerType != "" {
							jobs, err := c.jobs(ctx, repo, r)
							if err != nil {
								warnings = append(warnings, err.Error())
							}
							r.Jobs = jobs
						}
						local = append(local, r)
					}
				}
				if command == "runners" {
					rs, err := c.runners(ctx, "repos/"+repo)
					mu.Lock()
					s.Runners = append(s.Runners, rs...)
					mu.Unlock()
					if err != nil {
						warnings = append(warnings, err.Error())
					}
				}
				mu.Lock()
				s.Runs = append(s.Runs, local...)
				s.Warnings = append(s.Warnings, warnings...)
				mu.Unlock()
			}
		}()
	}
	for _, repo := range repos {
		tasks <- repo
	}
	close(tasks)
	wg.Wait()
	if command == "runners" {
		org := cfg.Organization
		if org == "" && cfg.Repo != "" {
			org = strings.Split(cfg.Repo, "/")[0]
		}
		rs, err := c.runners(ctx, "orgs/"+org)
		s.Runners = append(s.Runners, rs...)
		if err != nil {
			s.Warnings = append(s.Warnings, err.Error())
		}
	}
	sort.Slice(s.Runs, func(i, j int) bool {
		a, b := s.Runs[i], s.Runs[j]
		if a.CreatedAt.Equal(b.CreatedAt) {
			if a.Repository == b.Repository {
				return a.ID < b.ID
			}
			return a.Repository < b.Repository
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})
	sort.Slice(s.Runners, func(i, j int) bool {
		if s.Runners[i].Scope == s.Runners[j].Scope {
			return s.Runners[i].ID < s.Runners[j].ID
		}
		return s.Runners[i].Scope < s.Runners[j].Scope
	})
	sort.Strings(s.Warnings)
	s.FinishedAt = time.Now().UTC()
	return s, nil
}
