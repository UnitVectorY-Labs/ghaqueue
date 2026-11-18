package app

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestJSONOutput(t *testing.T) {
	var out bytes.Buffer
	if err := writeJSON(&out, Snapshot{Runs: []RunInfo{{}}, Warnings: nil}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.HasSuffix(got, "\n") || !strings.Contains(got, "\n  \"runs\": [") || !strings.Contains(got, `"warnings": []`) || strings.Contains(got, `"jobs"`) || strings.Contains(got, `"runners"`) {
		t.Fatalf("unexpected JSON output: %s", got)
	}
	if err := writeJSON(shortWriter{}, Snapshot{}); err == nil {
		t.Fatal("lost JSON output failure")
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return 0, nil }
func TestOutputFailures(t *testing.T) {
	w := &checkedWriter{w: shortWriter{}}
	render(w, Snapshot{}, Config{}, "overview")
	if w.err != io.ErrShortWrite {
		t.Fatalf("lost output failure: %v", w.err)
	}
}
func TestVersionAndHelpWithoutScope(t *testing.T) {
	t.Chdir(t.TempDir())
	var out, errOut bytes.Buffer
	if code := Run([]string{"--version"}, "1.2.3", &out, &errOut); code != 0 || !bytes.Contains(out.Bytes(), []byte("v1.2.3")) {
		t.Fatalf("version %d %s %s", code, &out, &errOut)
	}
	if code := Run([]string{"--help"}, "dev", &out, &errOut); code != 0 {
		t.Fatalf("help %d", code)
	}
}
