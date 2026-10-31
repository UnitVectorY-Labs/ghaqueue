package app

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestTerminalColorPolicy(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "")
	if terminalProfile(true, Config{}) != termenv.ANSI256 {
		t.Fatal("terminal should have color")
	}
	if terminalProfile(false, Config{}) != termenv.Ascii {
		t.Fatal("pipe should be plain")
	}
	if terminalProfile(true, Config{NoColor: true}) != termenv.Ascii {
		t.Fatal("no-color ignored")
	}
	t.Setenv("NO_COLOR", "1")
	if terminalProfile(true, Config{}) != termenv.Ascii {
		t.Fatal("NO_COLOR ignored")
	}
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	if terminalProfile(true, Config{}) != termenv.Ascii {
		t.Fatal("dumb terminal should be plain")
	}
}
func presentationFixture() Snapshot {
	now := time.Date(2026, 10, 31, 15, 30, 0, 0, time.UTC)
	return Snapshot{Repositories: 100, StartedAt: now.Add(-3 * time.Second), FinishedAt: now, Runs: []RunInfo{
		{ID: 101, Attempt: 1, Repository: "acme/api", Name: "Build", Status: "in_progress", CreatedAt: now.Add(-30 * time.Minute), HTMLURL: "https://github.com/acme/api/actions/runs/101", Jobs: []Job{
			{ID: 201, Name: "Build image", Status: "queued", CreatedAt: now.Add(-23 * time.Minute), Labels: []string{"self-hosted", "linux", "build"}, HTMLURL: "https://github.com/acme/api/actions/runs/101/job/201"},
			{ID: 202, Name: "Test", Status: "in_progress", StartedAt: now.Add(-2 * time.Minute), Labels: []string{"ubuntu-latest"}, RunnerName: "GitHub Actions 42"},
		}},
		{ID: 102, Repository: "acme/web", Name: "Build", Status: "in_progress", Jobs: []Job{
			{ID: 203, Name: "Build image", Status: "queued", CreatedAt: now.Add(-18 * time.Minute), Labels: []string{"self-hosted", "linux", "build"}},
			{ID: 204, Name: "Lint", Status: "completed", CompletedAt: now.Add(-time.Minute), Labels: []string{"ubuntu-latest"}},
		}},
		{ID: 103, Repository: "acme/worker", Name: "Deploy", Status: "waiting"},
	}}
}
func TestPresentationPriorityAndNarrowOutput(t *testing.T) {
	s := presentationFixture()
	var b bytes.Buffer
	render(&b, s, Config{Organization: "acme"}, "overview")
	text := b.String()
	if strings.Contains(text, "\x1b") || strings.Contains(text, "\r") {
		t.Fatal("redirected output contains terminal controls")
	}
	if !strings.Contains(text, "2 pending") || !strings.Contains(text, "1 running") || !strings.Contains(text, "23m") {
		t.Fatal(text)
	}
	if strings.Index(text, "2 pending") > strings.Index(text, "Runner demand") || strings.Index(text, "Runner demand") > strings.Index(text, "Repositories") {
		t.Fatal("summary must precede demand and repository sections")
	}
	if strings.Contains(text, "DONE") || strings.Contains(text, "LAST FINISH") {
		t.Fatal("overview should prioritize live demand")
	}
	if !strings.Contains(text, "no jobs visible yet") {
		t.Fatal("unmaterialized work hidden")
	}
	b.Reset()
	term := newTerminal(&b, Config{})
	term.width = 45
	renderTerminal(term, s, Config{}, "jobs")
	for _, line := range strings.Split(b.String(), "\n") {
		if lipgloss.Width(line) > 45 {
			t.Fatalf("line exceeds terminal: %q", line)
		}
	}
	if strings.Contains(b.String(), "https://") {
		t.Fatal("links should require details")
	}
	b.Reset()
	render(&b, s, Config{Details: true}, "jobs")
	if !strings.Contains(b.String(), "https://github.com/acme/api") {
		t.Fatal("details omitted link")
	}
}
func TestPartialAndEmptyViews(t *testing.T) {
	var b bytes.Buffer
	s := presentationFixture()
	s.Warnings = []string{"permission denied"}
	render(&b, s, Config{}, "overview")
	if strings.Index(b.String(), "Partial snapshot") > strings.Index(b.String(), "2 pending") {
		t.Fatal("partial warning must precede totals")
	}
	b.Reset()
	render(&b, Snapshot{}, Config{}, "runners")
	if strings.Contains(b.String(), "workflow runs") {
		t.Fatal("runner emptiness should not be about workflow runs")
	}
}
func TestProgressFormattingAndSuppression(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var b bytes.Buffer
	term := newTerminal(&b, Config{})
	term.width = 80
	for _, done := range []int{0, 15, 25} {
		line := term.progressLine(scanEvent{Phase: "Scanning", Done: done, Total: 25}, time.Second)
		if lipgloss.Width(line) > 80 {
			t.Fatal("progress overflow")
		}
		if !strings.Contains(line, "/25") || !strings.Contains(line, "%") {
			t.Fatal(line)
		}
		if ansi.Strip(line) != line {
			t.Fatal("NO_COLOR bar contains ANSI")
		}
	}
	p := startProgress(&b, &b, Config{})
	p.update(scanEvent{Phase: "Scanning", Done: 1, Total: 1})
	p.finish()
	p.finish()
	if b.Len() != 0 {
		t.Fatal("progress must be suppressed for pipes")
	}
}

func TestProgressCompletionLifecycle(t *testing.T) {
	var b bytes.Buffer
	p := &scanProgress{terminal: newTerminal(&b, Config{}), events: make(chan scanEvent, 16), stop: make(chan struct{}), finished: make(chan struct{})}
	go p.run()
	for i := 0; i <= 10; i++ {
		p.update(scanEvent{Phase: "Scanning", Done: i, Total: 10})
	}
	p.finish()
	p.finish()
	text := b.String()
	if !strings.Contains(text, "10/10") || !strings.Contains(text, "100%") || !strings.HasSuffix(text, "\n") {
		t.Fatalf("lost final completion: %q", text)
	}
}
func TestOverviewBoundsAndDemandCounts(t *testing.T) {
	s := presentationFixture()
	for i := 0; i < 12; i++ {
		s.Runs = append(s.Runs, RunInfo{Repository: strings.Repeat("long", 35) + string(rune('a'+i)), Jobs: []Job{{Status: "queued", Labels: []string{strings.Repeat("wide", 20) + string(rune('a'+i))}}}})
	}
	var b bytes.Buffer
	term := newTerminal(&b, Config{})
	term.width = 80
	renderTerminal(term, s, Config{}, "overview")
	if !strings.Contains(b.String(), "Showing 5 of") || !strings.Contains(b.String(), "Showing 8 of") {
		t.Fatal("overview should bound large organization output")
	}
	for _, line := range strings.Split(b.String(), "\n") {
		if lipgloss.Width(line) > 80 {
			t.Fatalf("wide overview row: %q", line)
		}
	}
	pools := activeGroups(presentationFixture(), Config{}, "pools")
	for _, g := range pools {
		if g.Name == "ubuntu-latest" && len(g.Repos) != 1 {
			t.Fatal("completed-only repositories should not count as active demand")
		}
	}
}
