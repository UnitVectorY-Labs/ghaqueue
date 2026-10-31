package app

import (
	"bytes"
	"io"
	"testing"
)

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
