package app

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
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
			g.Repos[r.Repository] = true
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
func printGroups(w io.Writer, s Snapshot, c Config, by string) {
	t := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(t, "GROUP\tREPOS\tPENDING\tRUNNING\tDONE*\tOLDEST PENDING\tLAST START*\tLAST FINISH*")
	for _, g := range groups(s, c, by) {
		fmt.Fprintf(t, "%s\t%d\t%d\t%d\t%d\t%s\t%s\t%s\n", clean(g.Name), len(g.Repos), g.Pending, g.Running, g.Completed, age(s.FinishedAt, g.Oldest), age(s.FinishedAt, g.LatestStart), age(s.FinishedAt, g.LatestFinish))
	}
	t.Flush()
	fmt.Fprintln(w, "* Done/start/finish cover only jobs in the collected active runs; ages are not throughput or proof of progress.")
}
func render(w io.Writer, s Snapshot, c Config, command string) {
	fmt.Fprintf(w, "%s  |  %d repositories  |  collected in %s\n", s.FinishedAt.Format(time.RFC3339), s.Repositories, s.FinishedAt.Sub(s.StartedAt).Round(time.Millisecond))
	switch command {
	case "overview":
		counts := map[string]int{}
		pending, running := 0, 0
		for _, r := range s.Runs {
			if c.Status == "" || r.Status == c.Status {
				counts[r.Status]++
			}
			for _, j := range r.Jobs {
				if !matchesJob(j, c) {
					continue
				}
				if j.Status == "in_progress" {
					running++
				} else if active(j.Status) {
					pending++
				}
			}
		}
		fmt.Fprintf(w, "Runs: queued=%d in_progress=%d waiting=%d pending=%d requested=%d\nJobs: %d pending, %d running\n\n", counts["queued"], counts["in_progress"], counts["waiting"], counts["pending"], counts["requested"], pending, running)
		printGroups(w, s, c, "repos")
		fmt.Fprintln(w, "\nUse jobs for individual work, runs for individual workflows, workflows for shared names, pools for shared labels, runners for inventory.")
	case "pools":
		printGroups(w, s, c, "pools")
		fmt.Fprintln(w, "Label sets are demand groups, not disjoint runner capacity pools.")
	case "jobs":
		t := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(t, "REPOSITORY / WORKFLOW\tJOB\tSTATUS\tWAIT / RUN AGE\tLABELS\tRUNNER\tURL")
		for _, r := range s.Runs {
			for _, j := range r.Jobs {
				if !active(j.Status) || !matchesJob(j, c) {
					continue
				}
				at := j.CreatedAt
				if j.Status == "in_progress" {
					at = j.StartedAt
				}
				fmt.Fprintf(t, "%s / %s\t%s\t%s\t%s\t%s\t%s\t%s\n", clean(r.Repository), clean(r.Name), clean(j.Name), clean(j.Status), age(s.FinishedAt, at), clean(pool(j)), clean(j.RunnerName), clean(j.HTMLURL))
			}
		}
		t.Flush()
	case "workflows":
		printGroups(w, s, c, "workflows")
		fmt.Fprintln(w, "Workflow names are grouped across repositories; identical names do not guarantee identical definitions.")
	case "runs", "failures":
		t := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(t, "REPOSITORY\tWORKFLOW\tRUN / ATTEMPT\tSTATUS\tCONCLUSION\tRUN AGE\tUPDATED AGO\tURL")
		for _, r := range s.Runs {
			if c.Status != "" && r.Status != c.Status {
				continue
			}
			if c.Label != "" || c.RunnerType != "" {
				found := false
				jobConfig := c
				jobConfig.Status = ""
				for _, j := range r.Jobs {
					found = found || matchesJob(j, jobConfig)
				}
				if !found {
					continue
				}
			}
			fmt.Fprintf(t, "%s\t%s\t%d / %d\t%s\t%s\t%s\t%s\t%s\n", clean(r.Repository), clean(r.Name), r.ID, r.Attempt, clean(r.Status), clean(r.Conclusion), age(s.FinishedAt, r.CreatedAt), age(s.FinishedAt, r.UpdatedAt), clean(r.HTMLURL))
		}
		t.Flush()
	case "runners":
		t := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(t, "SCOPE\tRUNNER\tOS\tSTATUS\tBUSY\tLABELS\tOBSERVED JOBS")
		for _, runner := range s.Runners {
			labels := []string{}
			match := c.Label == ""
			for _, l := range runner.Labels {
				labels = append(labels, l.Name)
				match = match || strings.EqualFold(l.Name, c.Label)
			}
			if !match || c.RunnerType == "unknown" {
				continue
			}
			sort.Strings(labels)
			jobs := []string{}
			for _, r := range s.Runs {
				for _, j := range r.Jobs {
					if j.RunnerID == runner.ID && j.Status == "in_progress" && matchesJob(j, c) {
						jobs = append(jobs, r.Repository+"/"+j.Name)
					}
				}
			}
			fmt.Fprintf(t, "%s\t%s\t%s\t%s\t%t\t%s\t%s\n", clean(runner.Scope), clean(runner.Name), clean(runner.OS), clean(runner.Status), runner.Busy, clean(strings.Join(labels, ",")), clean(strings.Join(jobs, ", ")))
		}
		t.Flush()
		fmt.Fprintln(w, "Inventory is self-hosted only. Busy runners may serve work outside this scan; scope access restrictions still apply.")
	}
	if len(s.Runs) == 0 {
		fmt.Fprintln(w, "No matching workflow runs observed.")
	}
}
