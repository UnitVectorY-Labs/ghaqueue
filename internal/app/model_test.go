package app

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestAggregationAndFilters(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	s := Snapshot{FinishedAt: now, Runs: []RunInfo{
		{Repository: "o/a", Name: "CI", Jobs: []Job{{Status: "queued", CreatedAt: now.Add(-time.Hour), Labels: []string{"linux", "self-hosted"}}, {Status: "in_progress", StartedAt: now.Add(-time.Minute), Labels: []string{"self-hosted", "linux"}}}},
		{Repository: "o/b", Name: "CI", Jobs: []Job{{Status: "waiting", CreatedAt: now.Add(-2 * time.Hour), Labels: []string{"linux", "self-hosted"}}, {Status: "queued", Labels: []string{"ubuntu-latest"}}}},
	}}
	a := groups(s, Config{Label: "linux"}, "pools")
	if len(a) != 1 || a[0].Pending != 2 || a[0].Running != 1 || len(a[0].Repos) != 2 || !a[0].Oldest.Equal(now.Add(-2*time.Hour)) {
		t.Fatalf("bad pool aggregate: %+v", a)
	}
	a = groups(s, Config{}, "workflows")
	if len(a) != 1 || a[0].Pending != 3 {
		t.Fatalf("bad workflow aggregate: %+v", a)
	}
	if runnerType(Job{Labels: []string{"ubuntu-latest"}}) != "unknown" {
		t.Fatal("must not infer hosted capacity from labels")
	}
	if age(now, time.Time{}) != "unknown" || age(now, now.Add(time.Hour)) != "0s" {
		t.Fatal("invalid age handling")
	}
	var b bytes.Buffer
	render(&b, s, Config{Label: "linux"}, "jobs")
	if strings.Contains(b.String(), "ubuntu-latest") {
		t.Fatal("label filter ignored")
	}
}
func TestUnmaterializedRunAndTerminalSafety(t *testing.T) {
	var b bytes.Buffer
	render(&b, Snapshot{Runs: []RunInfo{{Name: "no jobs yet", Status: "queued"}}}, Config{}, "overview")
	if !strings.Contains(b.String(), "queued=1") || !strings.Contains(b.String(), "0 pending") {
		t.Fatal(b.String())
	}
	if clean("job\x1b[31m\nnext\tcell") != "job [31m next cell" {
		t.Fatal("terminal controls escaped")
	}
}
func TestRepositoryRemote(t *testing.T) {
	for in, want := range map[string]string{"git@github.com:org/repo.git": "org/repo", "https://github.com/org/repo.git": "org/repo", "ssh://git@github.com/org/repo.git": "org/repo", "https://github.com.evil/org/repo": "", "/local/repo": "", "https://github.com/org/repo/extra": ""} {
		if got := repositoryFromRemote(in); got != want {
			t.Errorf("%q: %q != %q", in, got, want)
		}
	}
}

func TestRunStatusDoesNotFilterJobLabels(t *testing.T) {
	s := Snapshot{Runs: []RunInfo{{ID: 1, Name: "Approval", Status: "waiting", Jobs: []Job{{Status: "queued", Labels: []string{"linux"}}}}}}
	var b bytes.Buffer
	render(&b, s, Config{Status: "waiting", Label: "linux"}, "runs")
	if !strings.Contains(b.String(), "Approval") {
		t.Fatal("run label filter incorrectly used job status")
	}
}
