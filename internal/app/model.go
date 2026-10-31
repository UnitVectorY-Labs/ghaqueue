package app

import (
	"sort"
	"strings"
	"time"
)

type RunInfo struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	HTMLURL    string    `json:"html_url"`
	Attempt    int       `json:"run_attempt"`
	Repository string    `json:"repository_name"`
	Jobs       []Job     `json:"jobs,omitempty"`
}
type Job struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	CreatedAt   time.Time `json:"created_at"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Labels      []string  `json:"labels"`
	RunnerID    int64     `json:"runner_id"`
	RunnerName  string    `json:"runner_name"`
	RunnerGroup string    `json:"runner_group_name"`
	HTMLURL     string    `json:"html_url"`
}
type Runner struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	OS     string `json:"os"`
	Status string `json:"status"`
	Busy   bool   `json:"busy"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Scope string `json:"scope"`
}
type Snapshot struct {
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	Repositories int       `json:"repositories"`
	Runs         []RunInfo `json:"runs"`
	Runners      []Runner  `json:"runners,omitempty"`
	Warnings     []string  `json:"warnings"`
}

func active(s string) bool { return s != "completed" }
func pool(j Job) string {
	a := append([]string(nil), j.Labels...)
	sort.Strings(a)
	if len(a) == 0 {
		return "unknown"
	}
	return strings.Join(a, ",")
}
func runnerType(j Job) string {
	for _, l := range j.Labels {
		if strings.EqualFold(l, "self-hosted") {
			return "self-hosted"
		}
	}
	// Labels alone cannot reliably classify custom/scale-set runners.
	return "unknown"
}
func matchesJob(j Job, c Config) bool {
	if c.Status != "" && j.Status != c.Status {
		return false
	}
	if c.RunnerType != "" && runnerType(j) != c.RunnerType {
		return false
	}
	if c.Label != "" {
		for _, l := range j.Labels {
			if strings.EqualFold(l, c.Label) {
				return true
			}
		}
		return false
	}
	return true
}
func matchesWorkflow(r RunInfo, s string) bool {
	return s == "" || strings.Contains(strings.ToLower(r.Name), strings.ToLower(s)) || strings.Contains(strings.ToLower(r.Path), strings.ToLower(s))
}
func age(now, t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	return d.Round(time.Second).String()
}
