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
	s, err := collect(context.Background(), &c, Config{Repo: "o/r", Workers: 2, Since: "24h"}, "overview")
	if err != nil || len(s.Runs) != 1 || len(s.Runs[0].Jobs) != 1 || len(s.Warnings) != 1 {
		t.Fatalf("bad partial snapshot %+v %v", s, err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if e := c.get(ctx, "/anything", new(any)); e == nil {
		t.Fatal("ignored cancellation")
	}
}
