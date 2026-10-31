package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunsPaginationAndLimit(t *testing.T) {
	for _, total := range []int{101, 1001} {
		t.Run(fmt.Sprint(total), func(t *testing.T) {
			pages := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pages++
				if r.URL.Query().Get("status") != "queued" || r.Header.Get("Authorization") != "Bearer example" {
					t.Error("request contract")
				}
				n := 100
				if total == 101 && pages == 2 {
					n = 1
				}
				a := make([]RunInfo, n)
				for i := range a {
					a[i].ID = int64((pages-1)*100 + i)
				}
				json.NewEncoder(w).Encode(map[string]any{"total_count": total, "workflow_runs": a})
			}))
			defer srv.Close()
			c := client{http: srv.Client(), base: srv.URL, token: "example"}
			runs, err := c.runs(context.Background(), "o/r", "queued")
			if total == 101 {
				if err != nil || len(runs) != 101 || pages != 2 {
					t.Fatalf("pagination %d %d %v", len(runs), pages, err)
				}
			} else if err == nil || len(runs) != 1000 {
				t.Fatalf("missing truncation warning: %d %v", len(runs), err)
			}
		})
	}
}
func TestCollectionPartialAndAttempt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/attempts/2/jobs"):
			fmt.Fprint(w, `{"jobs":[{"id":12,"status":"queued","name":"build"}]}`)
		case strings.HasSuffix(r.URL.Path, "/runs"):
			switch r.URL.Query().Get("status") {
			case "":
				fmt.Fprint(w, `{"total_count":501,"workflow_runs":[]}`)
			case "queued":
				fmt.Fprint(w, `{"total_count":1,"workflow_runs":[{"id":1,"name":"CI","status":"queued","run_attempt":2}]}`)
			case "waiting":
				w.WriteHeader(403)
			default:
				fmt.Fprint(w, `{"total_count":0,"workflow_runs":[]}`)
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := client{http: srv.Client(), base: srv.URL}
	events := []scanEvent{}
	s, err := collect(context.Background(), &c, Config{Repo: "o/r", Workers: 2, Since: "24h"}, "overview", func(e scanEvent) { events = append(events, e) })
	if len(events) != 3 || events[0].Phase != "Discovering repositories" || events[1].Done != 0 || events[2].Done != 1 || events[2].Total != 1 || events[2].Warnings != 1 {
		t.Fatalf("incorrect scan progress: %+v", events)
	}
	if err != nil || len(s.Runs) != 1 || len(s.Runs[0].Jobs) != 1 || len(s.Warnings) != 1 || s.APIRequests != 7 {
		t.Fatalf("bad partial snapshot %+v %v", s, err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if e := c.get(ctx, "/anything", new(any)); e == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestAdaptiveLiveRunRequests(t *testing.T) {
	for _, total := range []int{0, 1, 100, 101, 500, 501} {
		t.Run(fmt.Sprint(total), func(t *testing.T) {
			calls := 0
			statuses := map[string]bool{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				q := r.URL.Query()
				status := q.Get("status")
				statuses[status] = true
				if q.Get("exclude_pull_requests") != "true" || q.Get("created") != "" {
					t.Error("must omit unused payload without imposing a time cutoff")
				}
				if total > fullHistoryLimit && status != "" {
					runs := []RunInfo{}
					if status == "queued" {
						runs = append(runs, RunInfo{ID: 999, Status: "queued", CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)})
					}
					json.NewEncoder(w).Encode(map[string]any{"total_count": len(runs), "workflow_runs": runs})
					return
				}
				var page int
				fmt.Sscan(q.Get("page"), &page)
				count := min(100, max(0, total-(page-1)*100))
				runs := make([]RunInfo, count)
				for i := range runs {
					runs[i] = RunInfo{ID: int64((page-1)*100 + i + 1), Status: "completed"}
					if int(runs[i].ID) == total {
						runs[i].Status = "waiting"
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"total_count": total, "workflow_runs": runs})
			}))
			defer srv.Close()
			c := client{http: srv.Client(), base: srv.URL}
			runs, warnings := c.liveRuns(context.Background(), "o/r")
			want := max(1, (total+99)/100)
			if total > fullHistoryLimit {
				want = 6
			}
			if calls != want || len(warnings) != 0 {
				t.Fatalf("got %d calls, wanted %d; warnings=%v", calls, want, warnings)
			}
			if total == 0 {
				if len(runs) != 0 {
					t.Fatal("empty history produced active work")
				}
			} else if len(runs) != 1 {
				t.Fatalf("lost active run: %+v", runs)
			}
			if total > fullHistoryLimit {
				for _, status := range liveStatuses {
					if !statuses[status] {
						t.Fatalf("missed status %s", status)
					}
				}
				if runs[0].ID != 999 {
					t.Fatal("old queued run was missed")
				}
			}
		})
	}
}
func TestRunsViewAvoidsUnneededJobReads(t *testing.T) {
	for _, label := range []string{"", "linux"} {
		t.Run("label="+label, func(t *testing.T) {
			runReads, jobReads := 0, 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/runs") {
					runReads++
					if r.URL.Query().Get("status") != "queued" {
						t.Error("explicit run status should be pushed to GitHub")
					}
					fmt.Fprint(w, `{"total_count":1,"workflow_runs":[{"id":1,"name":"CI","status":"queued","run_attempt":2}]}`)
				} else if strings.Contains(r.URL.Path, "/attempts/2/jobs") {
					jobReads++
					fmt.Fprint(w, `{"jobs":[{"id":2,"status":"queued","labels":["linux"]}]}`)
				} else {
					t.Error("unexpected read " + r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer srv.Close()
			c := client{http: srv.Client(), base: srv.URL}
			s, err := collect(context.Background(), &c, Config{Repo: "o/r", Workers: 1, Status: "queued", Label: label, Since: "24h"}, "runs")
			if err != nil || len(s.Warnings) > 0 || len(s.Runs) != 1 || runReads != 1 || s.APIRequests != int64(runReads+jobReads) {
				t.Fatalf("unexpected result: %+v %v reads=%d", s, err, runReads)
			}
			if (label == "" && jobReads != 0) || (label != "" && jobReads != 1) {
				t.Fatalf("job reads=%d for label=%q", jobReads, label)
			}
		})
	}
}
