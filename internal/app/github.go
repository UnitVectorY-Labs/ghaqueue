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
	"sync/atomic"
	"time"
)

type client struct {
	http     *http.Client
	token    string
	base     string
	requests atomic.Int64
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
	c.requests.Add(1)
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

// Small histories cost at most five reads; large histories use the complete
// active-status queries rather than assuming recent runs contain all queued work.
const fullHistoryLimit = 500

var liveStatuses = []string{"queued", "in_progress", "waiting", "pending", "requested"}

func (c *client) runPage(ctx context.Context, repo, status string, page int) ([]RunInfo, int, error) {
	var data struct {
		Total int       `json:"total_count"`
		Runs  []RunInfo `json:"workflow_runs"`
	}
	q := url.Values{"per_page": {"100"}, "page": {strconv.Itoa(page)}, "exclude_pull_requests": {"true"}}
	if status != "" {
		q.Set("status", status)
	}
	if err := c.get(ctx, "/repos/"+repo+"/actions/runs?"+q.Encode(), &data); err != nil {
		return nil, 0, err
	}
	for i := range data.Runs {
		data.Runs[i].Repository = repo
	}
	return data.Runs, data.Total, nil
}
func (c *client) runs(ctx context.Context, repo, status string) ([]RunInfo, error) {
	runs := []RunInfo{}
	for page := 1; page <= 10; page++ {
		batch, total, err := c.runPage(ctx, repo, status, page)
		if err != nil {
			return runs, err
		}
		runs = append(runs, batch...)
		if len(batch) < 100 || len(runs) >= total {
			return runs, nil
		}
	}
	return runs, fmt.Errorf("%s status=%s: reached GitHub's 1,000 filtered-run limit; results may be incomplete", repo, status)
}
func (c *client) liveRuns(ctx context.Context, repo string) ([]RunInfo, []string) {
	first, total, err := c.runPage(ctx, repo, "", 1)
	if err != nil {
		return nil, []string{err.Error()}
	}
	result := []RunInfo{}
	warnings := []string{}
	if total <= fullHistoryLimit {
		all := append([]RunInfo(nil), first...)
		for page := 2; len(all) < total && len(first) == 100; page++ {
			var batch []RunInfo
			batch, total, err = c.runPage(ctx, repo, "", page)
			if err != nil {
				warnings = append(warnings, err.Error())
				break
			}
			all = append(all, batch...)
			if len(batch) < 100 {
				break
			}
			// If the history grew during collection, avoid an unbounded history scan.
			if total > fullHistoryLimit {
				return c.liveStatusRuns(ctx, repo)
			}
		}
		for _, r := range all {
			if active(r.Status) {
				result = append(result, r)
			}
		}
		return result, warnings
	}
	return c.liveStatusRuns(ctx, repo)
}
func (c *client) liveStatusRuns(ctx context.Context, repo string) ([]RunInfo, []string) {
	result := []RunInfo{}
	warnings := []string{}
	for _, status := range liveStatuses {
		runs, err := c.runs(ctx, repo, status)
		result = append(result, runs...)
		if err != nil {
			warnings = append(warnings, err.Error())
		}
	}
	return result, warnings
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

type scanEvent struct {
	Phase                       string
	Done, Total, Runs, Warnings int
}

func collect(ctx context.Context, c *client, cfg Config, command string, observers ...func(scanEvent)) (Snapshot, error) {
	notify := func(e scanEvent) {
		for _, observe := range observers {
			observe(e)
		}
	}
	notify(scanEvent{Phase: "Discovering repositories"})
	s := Snapshot{StartedAt: time.Now().UTC(), Runs: []RunInfo{}, Warnings: []string{}}
	repos, e := c.repos(ctx, cfg)
	if e != nil {
		s.APIRequests = c.requests.Load()
		return s, e
	}
	s.Repositories = len(repos)
	type result struct {
		runs     []RunInfo
		runners  []Runner
		warnings []string
	}
	results := make(chan result)
	tasks := make(chan string)
	var wg sync.WaitGroup
	since, _ := time.ParseDuration(cfg.Since)
	cutoff := s.StartedAt.Add(-since)
	notify(scanEvent{Phase: "Scanning", Total: len(repos)})
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for repo := range tasks {
				local := []RunInfo{}
				inventory := []Runner{}
				warnings := []string{}
				runs := []RunInfo{}
				if command == "failures" {
					for _, status := range []string{"failure", "timed_out", "action_required"} {
						batch, err := c.runs(ctx, repo, status)
						runs = append(runs, batch...)
						if err != nil {
							warnings = append(warnings, err.Error())
						}
					}
				} else if command == "runs" && cfg.Status != "" {
					if cfg.Status != "completed" {
						batch, err := c.runs(ctx, repo, cfg.Status)
						runs = append(runs, batch...)
						if err != nil {
							warnings = append(warnings, err.Error())
						}
					}
				} else {
					batch, issues := c.liveRuns(ctx, repo)
					runs = batch
					warnings = append(warnings, issues...)
				}
				seen := map[int64]bool{}
				for _, r := range runs {
					if seen[r.ID] || !matchesWorkflow(r, cfg.Workflow) {
						continue
					}
					seen[r.ID] = true
					if command == "failures" {
						if r.UpdatedAt.Before(cutoff) || (cfg.Status != "" && r.Status != cfg.Status) {
							continue
						}
					} else if !active(r.Status) {
						continue
					}
					needJobs := command != "runs" && command != "failures" || cfg.Label != "" || cfg.RunnerType != ""
					if needJobs {
						jobs, err := c.jobs(ctx, repo, r)
						if err != nil {
							warnings = append(warnings, err.Error())
						}
						r.Jobs = jobs
					}
					local = append(local, r)
				}

				if command == "runners" {
					rs, err := c.runners(ctx, "repos/"+repo)
					inventory = append(inventory, rs...)
					if err != nil {
						warnings = append(warnings, err.Error())
					}
				}
				results <- result{local, inventory, warnings}
			}
		}()
	}
	go func() {
		for _, repo := range repos {
			tasks <- repo
		}
		close(tasks)
		wg.Wait()
		close(results)
	}()
	done := 0
	for result := range results {
		s.Runs = append(s.Runs, result.runs...)
		s.Runners = append(s.Runners, result.runners...)
		s.Warnings = append(s.Warnings, result.warnings...)
		done++
		notify(scanEvent{Phase: "Scanning", Done: done, Total: len(repos), Runs: len(s.Runs), Warnings: len(s.Warnings)})
	}
	if command == "runners" {
		notify(scanEvent{Phase: "Reading organization runners", Done: len(repos), Total: len(repos), Runs: len(s.Runs), Warnings: len(s.Warnings)})
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
	s.APIRequests = c.requests.Load()
	s.FinishedAt = time.Now().UTC()
	return s, nil
}
