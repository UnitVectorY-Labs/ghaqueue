package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

func Run(args []string, version string, out, errOut io.Writer) int {
	c, command, v, err := parse(args, errOut)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}
	if v {
		if version == "dev" {
			if b, ok := debug.ReadBuildInfo(); ok && b.Main.Version != "" && b.Main.Version != "(devel)" {
				version = b.Main.Version
			}
		}
		if len(version) > 0 && version[0] >= '0' && version[0] <= '9' {
			version = "v" + version
		}
		fmt.Fprintf(out, "ghaqueue version %s (%s, %s/%s)\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return 0
	}
	timeout, _ := time.ParseDuration(c.Timeout)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	s, err := collect(ctx, newClient(ctx), c, command)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}
	// JSON carries the same selected rows as terminal views while retaining completed
	// jobs for activity context in aggregate views.
	if c.JSON {
		jobConfig := c
		if command == "runs" || command == "failures" {
			jobConfig.Status = ""
		}
		filtered := []RunInfo{}
		for _, r := range s.Runs {
			if (command == "runs" || command == "failures") && c.Status != "" && r.Status != c.Status {
				continue
			}
			jobs := []Job{}
			for _, j := range r.Jobs {
				if matchesJob(j, jobConfig) && (command != "jobs" || active(j.Status)) {
					jobs = append(jobs, j)
				}
			}
			r.Jobs = jobs
			if (c.Label != "" || c.RunnerType != "") && len(jobs) == 0 {
				continue
			}
			filtered = append(filtered, r)
		}
		s.Runs = filtered
		runners := []Runner{}
		for _, r := range s.Runners {
			match := c.Label == ""
			for _, l := range r.Labels {
				match = match || strings.EqualFold(l.Name, c.Label)
			}
			if match && c.RunnerType != "unknown" {
				runners = append(runners, r)
			}
		}
		s.Runners = runners
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		err = enc.Encode(s)
	} else {
		checked := &checkedWriter{w: out}
		render(checked, s, c, command)
		err = checked.err
	}
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}
	for _, w := range s.Warnings {
		fmt.Fprintln(errOut, "warning:", clean(w))
	}
	if len(s.Warnings) > 0 {
		fmt.Fprintln(errOut, "Partial snapshot; totals may be incomplete.")
		return 2
	}
	return 0
}

type checkedWriter struct {
	w   io.Writer
	err error
}

func (w *checkedWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, e := w.w.Write(p)
	if e == nil && n < len(p) {
		e = io.ErrShortWrite
	}
	w.err = e
	return n, e
}
