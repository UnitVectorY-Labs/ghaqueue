package app

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPrecedence(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".ghaqueue", []byte("organization: configured\nrepo: old\nworkers: 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GHAQUEUE_ORG", "environment")
	t.Setenv("GHAQUEUE_REPO", "")
	t.Setenv("GHAQUEUE_WORKERS", "3")
	c, cmd, _, err := parse([]string{"--workers", "4", "pools", "--org", "flagged"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.Organization != "flagged" || c.Repo != "" || c.Workers != 4 || cmd != "pools" {
		t.Fatalf("bad precedence: %+v %s", c, cmd)
	}
	if err := os.Mkdir("child", 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join("child"))
	p, err := discoverConfig()
	if err != nil || filepath.Base(p) != ".ghaqueue" {
		t.Fatalf("ancestor config: %q %v", p, err)
	}
}
func TestInvalidConfigAndArguments(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GHAQUEUE_ORG", "org")
	for _, args := range [][]string{{"--workers", "0"}, {"--since", "yesterday"}, {"--repo", "a/b/c"}, {"--runner-type", "github-hosted"}, {"--status", "bogus"}, {"jobs", "runs"}, {"overview", "overview"}, {"--json=invalid"}} {
		if _, _, _, e := parse(args, io.Discard); e == nil {
			t.Errorf("accepted %v", args)
		}
	}
	if err := os.WriteFile(".ghaqueue", []byte("organizaton: typo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, e := parse(nil, io.Discard); e == nil {
		t.Fatal("accepted unknown YAML key")
	}
}
