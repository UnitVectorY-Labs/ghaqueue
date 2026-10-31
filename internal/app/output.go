package app

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"
)

// GitHub-controlled names must not inject terminal escape sequences or table rows.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

type group struct {
	Name                        string
	Pending, Running, Completed int
	Oldest                      time.Time
	LatestStart, LatestFinish   time.Time
	Repos                       map[string]bool
}

func groups(s Snapshot, c Config, by string) []group {
	m := map[string]*group{}
	for _, r := range s.Runs {
		for _, j := range r.Jobs {
			if !matchesJob(j, c) {
				continue
			}
			key := r.Repository
			if by == "workflows" {
				key = r.Name
			}
			if by == "pools" {
				key = pool(j)
			}
			g := m[key]
			if g == nil {
				g = &group{Name: key, Repos: map[string]bool{}}
				m[key] = g
			}
			if active(j.Status) {
				g.Repos[r.Repository] = true
			}
			if j.Status == "in_progress" {
				g.Running++
			} else if j.Status == "completed" {
				g.Completed++
			} else {
				g.Pending++
				if g.Oldest.IsZero() || (!j.CreatedAt.IsZero() && j.CreatedAt.Before(g.Oldest)) {
					g.Oldest = j.CreatedAt
				}
			}
			if j.StartedAt.After(g.LatestStart) {
				g.LatestStart = j.StartedAt
			}
			if j.CompletedAt.After(g.LatestFinish) {
				g.LatestFinish = j.CompletedAt
			}
		}
	}
	a := []group{}
	for _, g := range m {
		a = append(a, *g)
	}
	sort.Slice(a, func(i, j int) bool {
		if a[i].Pending != a[j].Pending {
			return a[i].Pending > a[j].Pending
		}
		return a[i].Name < a[j].Name
	})
	return a
}

func quantity(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
func displayAge(now, at time.Time) string {
	if at.IsZero() {
		return "unknown"
	}
	d := max(time.Duration(0), now.Sub(at))
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
}
func runSelected(r RunInfo, c Config, filterStatus bool) bool {
	if filterStatus && c.Status != "" && r.Status != c.Status {
		return false
	}
	if c.Label == "" && c.RunnerType == "" {
		return true
	}
	c.Status = ""
	for _, j := range r.Jobs {
		if matchesJob(j, c) {
			return true
		}
	}
	return false
}

type jobRow struct {
	run RunInfo
	job Job
}

func selectedJobs(s Snapshot, c Config) []jobRow {
	rows := []jobRow{}
	for _, r := range s.Runs {
		for _, j := range r.Jobs {
			if active(j.Status) && matchesJob(j, c) {
				rows = append(rows, jobRow{r, j})
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if (a.job.Status == "in_progress") != (b.job.Status == "in_progress") {
			return a.job.Status != "in_progress"
		}
		at, bt := a.job.CreatedAt, b.job.CreatedAt
		if a.job.Status == "in_progress" {
			at, bt = a.job.StartedAt, b.job.StartedAt
		}
		if at.IsZero() != bt.IsZero() {
			return !at.IsZero()
		}
		if !at.Equal(bt) {
			return at.Before(bt)
		}
		if a.run.Repository != b.run.Repository {
			return a.run.Repository < b.run.Repository
		}
		if a.run.ID != b.run.ID {
			return a.run.ID < b.run.ID
		}
		return a.job.ID < b.job.ID
	})
	return rows
}
func activeGroups(s Snapshot, c Config, by string) []group {
	result := []group{}
	for _, g := range groups(s, c, by) {
		if g.Pending+g.Running > 0 {
			result = append(result, g)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Pending != b.Pending {
			return a.Pending > b.Pending
		}
		if a.Running != b.Running {
			return a.Running > b.Running
		}
		return a.Name < b.Name
	})
	return result
}
func (t *terminal) groupTable(s Snapshot, c Config, by string, limit int) {
	rows := activeGroups(s, c, by)
	if len(rows) == 0 {
		t.line(t.dim.Render("  No matching active jobs."))
		return
	}
	count := len(rows)
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	label := "Repository"
	if by == "pools" {
		label = "Runner labels"
	}
	if by == "workflows" {
		label = "Workflow"
	}
	headers := []string{label, "Pending", "Running", "Oldest pending"}
	if by != "repos" {
		headers = append(headers, "Repos")
	}
	data := [][]string{}
	for _, g := range rows {
		if t.width < 72 {
			t.line("  " + t.heading.Render(clean(g.Name)))
			context := fmt.Sprintf("%d pending · %d running", g.Pending, g.Running)
			if g.Pending > 0 {
				context += " · oldest " + displayAge(s.FinishedAt, g.Oldest)
			}
			if by != "repos" {
				context += fmt.Sprintf(" · %d repos", len(g.Repos))
			}
			t.line("    " + context)
			continue
		}
		oldest := "—"
		if g.Pending > 0 {
			oldest = displayAge(s.FinishedAt, g.Oldest)
		}
		row := []string{clean(g.Name), fmt.Sprint(g.Pending), fmt.Sprint(g.Running), oldest}
		if by != "repos" {
			row = append(row, fmt.Sprint(len(g.Repos)))
		}
		data = append(data, row)
	}
	if t.width >= 72 {
		widths := make([]int, len(headers))
		for col, header := range headers {
			widths[col] = lipgloss.Width(header)
			for _, row := range data {
				widths[col] = max(widths[col], lipgloss.Width(row[col]))
			}
		}
		fixed := 2 * (len(headers) - 1)
		for _, width := range widths[1:] {
			fixed += width
		}
		widths[0] = min(widths[0], t.width-fixed)
		printRow := func(row []string, header bool) {
			cells := make([]string, len(row))
			for col, value := range row {
				style := t.dim
				if header {
					style = style.Bold(true)
				} else if col == 0 {
					style = t.heading
				} else if col == 1 {
					style = t.pending
				} else if col == 2 {
					style = t.running
				}
				cells[col] = style.Width(widths[col]).MaxWidth(widths[col]).Render(value)
				if col < len(row)-1 {
					cells[col] += "  "
				}
			}
			fmt.Fprintln(t.w, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
		}
		printRow(headers, true)
		for _, row := range data {
			printRow(row, false)
		}
	}

	if len(rows) < count {
		t.line(t.dim.Render(fmt.Sprintf("  Showing %d of %d groups; use %s for the full view.", len(rows), count, map[string]string{"repos": "jobs", "pools": "pools", "workflows": "workflows"}[by])))
	}
}
func (t *terminal) summary(s Snapshot, c Config) (pending, running int) {
	jobs := selectedJobs(s, c)
	repos := map[string]bool{}
	oldest := time.Time{}
	unknown := 0
	for _, row := range jobs {
		repos[row.run.Repository] = true
		if row.job.Status == "in_progress" {
			running++
			continue
		}
		pending++
		if row.job.CreatedAt.IsZero() {
			unknown++
			continue
		}
		if oldest.IsZero() || row.job.CreatedAt.Before(oldest) {
			oldest = row.job.CreatedAt
		}
	}
	t.line("  " + t.pending.Bold(true).Render(fmt.Sprintf("%d pending", pending)) + "  ·  " + t.running.Bold(true).Render(fmt.Sprintf("%d running", running)))
	if pending+running > 0 {
		t.line(t.dim.Render("  Active jobs across " + quantity(len(repos), "repository", "repositories")))
	}
	if pending > 0 {
		t.line("  Oldest pending: " + t.pending.Render(displayAge(s.FinishedAt, oldest)))
		if unknown > 0 {
			t.line(t.dim.Render(fmt.Sprintf("  Wait age unavailable for %d pending jobs", unknown)))
		}
	}
	return pending, running
}
func render(w io.Writer, s Snapshot, c Config, command string) {
	renderTerminal(newTerminal(w, c), s, c, command)
}
func renderTerminal(t *terminal, s Snapshot, c Config, command string) {
	scope := c.Organization
	if c.Repo != "" {
		scope = c.Repo
	}
	if scope == "" {
		scope = "GitHub Actions"
	}
	t.line(t.heading.Render("ghaqueue") + t.dim.Render(" ─ "+clean(scope)))
	t.line(t.dim.Render(quantity(s.Repositories, "repository", "repositories") + " scanned · " + command))
	filters := []string{}
	for _, f := range []struct{ name, value string }{{"workflow", c.Workflow}, {"label", c.Label}, {"type", c.RunnerType}, {"status", c.Status}} {
		if f.value != "" {
			filters = append(filters, f.name+"="+clean(f.value))
		}
	}
	if len(filters) > 0 {
		t.line(t.dim.Render("Filters: " + strings.Join(filters, " · ")))
	}
	if len(s.Warnings) > 0 {
		t.line(t.bad.Bold(true).Render(fmt.Sprintf("Partial snapshot · %d read warnings; counts may be incomplete", len(s.Warnings))))
	}
	t.line("")
	switch command {
	case "overview", "pools", "workflows", "jobs":
		pending, running := t.summary(s, c)
		noJobs, approvals := 0, 0
		for _, r := range s.Runs {
			if !runSelected(r, c, false) {
				continue
			}
			if len(r.Jobs) == 0 {
				noJobs++
			}
			if r.Status == "waiting" {
				approvals++
			}
		}
		if noJobs > 0 {
			t.line(t.pending.Render("  " + quantity(noJobs, "active workflow run has", "active workflow runs have") + " no jobs visible yet"))
		}
		if command == "overview" && approvals > 0 {
			t.line(t.pending.Render("  " + quantity(approvals, "workflow run is", "workflow runs are") + " waiting; check GitHub for approval or other gates"))
		}
		if pending+running == 0 && noJobs == 0 {
			t.line(t.good.Render("  No matching active jobs observed."))
		}
		switch command {
		case "overview":
			if pending+running > 0 {
				t.section("Runner demand")
				t.groupTable(s, c, "pools", 5)
				t.section("Repositories")
				t.groupTable(s, c, "repos", 8)
			}
		case "pools":
			t.section("Runner demand")
			t.groupTable(s, c, "pools", 0)
		case "workflows":
			t.section("Workflow demand")
			t.groupTable(s, c, "workflows", 0)
		case "jobs":
			t.jobBlocks(s, c)
		}
		if pending > 0 {
			t.line("")
			t.line(t.dim.Render("Pending age includes dependency and approval waits."))
		}
		if command == "pools" {
			t.line(t.dim.Render("Label sets describe demand; runner capacity can overlap."))
		}
		if command == "overview" {
			t.line("")
			t.line(t.dim.Render("Inspect: jobs · runs · workflows · pools · runners"))
		}
	case "runs", "failures":
		t.runBlocks(s, c, command)
	case "runners":
		t.runnerBlocks(s, c)
	}
	if c.Details {
		t.line("")
		t.line(t.dim.Render(fmt.Sprintf("Observed %s · scan took %s · %d API requests", s.FinishedAt.Format(time.RFC3339), s.FinishedAt.Sub(s.StartedAt).Round(time.Millisecond), s.APIRequests)))
	}
}
func (t *terminal) jobBlocks(s Snapshot, c Config) {
	rows := selectedJobs(s, c)
	for _, status := range []string{"pending", "running"} {
		count := 0
		for _, row := range rows {
			if (row.job.Status == "in_progress") == (status == "running") {
				count++
			}
		}
		if count == 0 {
			continue
		}
		t.section(fmt.Sprintf("%s jobs (%d)", strings.ToUpper(status[:1])+status[1:], count))
		for _, row := range rows {
			j, r := row.job, row.run
			if (j.Status == "in_progress") != (status == "running") {
				continue
			}
			at := j.CreatedAt
			suffix := "pending"
			if status == "running" {
				at = j.StartedAt
				suffix = "running"
			}
			ageText := displayAge(s.FinishedAt, at) + " " + suffix
			if at.IsZero() {
				ageText = "age unknown"
			}
			t.line("  " + t.status(j.Status) + "  " + t.heading.Render(clean(j.Name)) + "  " + t.dim.Render(ageText))
			t.line(t.dim.Render("    " + clean(r.Repository) + " / " + clean(r.Name)))
			context := pool(j)
			if j.RunnerName != "" {
				context = "runner: " + j.RunnerName + " · " + context
			}
			t.line(t.dim.Render("    " + clean(context)))
			if c.Details {
				t.line(t.dim.Render("    " + clean(j.HTMLURL)))
			}
		}
	}
}
func (t *terminal) runBlocks(s Snapshot, c Config, command string) {
	rows := []RunInfo{}
	for _, r := range s.Runs {
		if runSelected(r, c, true) {
			rows = append(rows, r)
		}
	}
	if command == "failures" {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].UpdatedAt.After(rows[j].UpdatedAt) })
	}
	if len(rows) == 0 {
		message := "No matching active workflow runs observed."
		if command == "failures" {
			message = "No matching recent failures observed."
		}
		t.line(t.good.Render("  " + message))
		return
	}
	title := "Active workflow runs"
	if command == "failures" {
		title = "Recent failures"
	}
	t.line(t.heading.Render(fmt.Sprintf("%s (%d)", title, len(rows))))
	if command == "failures" {
		t.line(t.dim.Render("Updated within " + c.Since + " · latest attempt per run"))
	}
	for _, r := range rows {
		status := r.Status
		if command == "failures" {
			status = r.Conclusion
		}
		t.line("")
		t.line("  " + t.status(status) + "  " + t.heading.Render(clean(r.Repository)+" / "+clean(r.Name)))
		context := fmt.Sprintf("#%d · attempt %d · run age %s", r.ID, r.Attempt, displayAge(s.FinishedAt, r.CreatedAt))
		if command == "failures" {
			context = fmt.Sprintf("#%d · attempt %d · updated %s ago", r.ID, r.Attempt, displayAge(s.FinishedAt, r.UpdatedAt))
		}
		t.line(t.dim.Render("    " + context))
		if c.Details {
			t.line(t.dim.Render("    " + clean(r.HTMLURL)))
		}
	}
	if !c.Details {
		t.line("")
		t.line(t.dim.Render("Use --details to include GitHub links."))
	}
}
func (t *terminal) runnerBlocks(s Snapshot, c Config) {
	rows := []Runner{}
	busy, offline := 0, 0
	for _, r := range s.Runners {
		match := c.Label == ""
		for _, l := range r.Labels {
			match = match || strings.EqualFold(l.Name, c.Label)
		}
		if !match || c.RunnerType == "unknown" {
			continue
		}
		rows = append(rows, r)
		if r.Busy {
			busy++
		}
		if r.Status == "offline" {
			offline++
		}
	}
	t.line("  " + t.heading.Render(fmt.Sprintf("%d self-hosted runner entries", len(rows))) + fmt.Sprintf(" · %d busy · %d offline", busy, offline))
	if len(rows) == 0 {
		t.line(t.dim.Render("  No matching runner inventory observed."))
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if (a.Status == "offline") != (b.Status == "offline") {
			return a.Status == "offline"
		}
		if a.Busy != b.Busy {
			return a.Busy
		}
		return a.Name < b.Name
	})
	for _, r := range rows {
		state := "idle"
		if r.Busy {
			state = "busy"
		}
		t.line("")
		t.line("  " + t.status(r.Status) + "  " + t.heading.Render(clean(r.Name)) + " · " + state)
		t.line(t.dim.Render("    " + clean(r.Scope) + " · " + clean(r.OS)))
		labels := []string{}
		for _, l := range r.Labels {
			labels = append(labels, l.Name)
		}
		sort.Strings(labels)
		t.line(t.dim.Render("    " + clean(strings.Join(labels, ", "))))
		for _, run := range s.Runs {
			for _, j := range run.Jobs {
				if j.RunnerID == r.ID && j.Status == "in_progress" && matchesJob(j, c) {
					t.line("    " + t.running.Render(clean(run.Repository)+" / "+clean(j.Name)))
				}
			}
		}
	}
	t.line("")
	t.line(t.dim.Render("Busy runners may serve work outside this scan. Scope entries are not a capacity total."))
}
