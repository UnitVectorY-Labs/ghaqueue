package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Organization string `yaml:"organization"`
	Repo         string `yaml:"repo"`
	Workflow     string `yaml:"workflow"`
	Label        string `yaml:"label"`
	RunnerType   string `yaml:"runner_type"`
	Status       string `yaml:"status"`
	Since        string `yaml:"since"`
	Workers      int    `yaml:"workers"`
	Timeout      string `yaml:"timeout"`
	JSON         bool   `yaml:"json"`
	NoColor      bool   `yaml:"no_color"`
	NoProgress   bool   `yaml:"no_progress"`
	Details      bool   `yaml:"details"`
}

func discoverConfig() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		p := filepath.Join(dir, ".ghaqueue")
		_, err := os.Stat(p)
		if err == nil {
			return p, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}
func parse(args []string, out io.Writer) (Config, string, bool, error) {
	c := Config{Workers: 8, Since: "24h", Timeout: "5m"}
	path, err := discoverConfig()
	if err != nil {
		return c, "", false, err
	}
	if path != "" {
		f, e := os.Open(path)
		if e != nil {
			return c, "", false, e
		}
		defer f.Close()
		d := yaml.NewDecoder(f)
		d.KnownFields(true)
		if e = d.Decode(&c); e != nil {
			return c, "", false, fmt.Errorf("%s: %w", path, e)
		}
	}
	f := flag.NewFlagSet("ghaqueue", flag.ContinueOnError)
	f.SetOutput(out)
	stringsFlags := map[string]*string{"org": &c.Organization, "repo": &c.Repo, "workflow": &c.Workflow, "label": &c.Label, "runner-type": &c.RunnerType, "status": &c.Status, "since": &c.Since, "timeout": &c.Timeout}
	descriptions := map[string]string{"org": "Scan organization", "repo": "Select OWNER/REPO (or repository name with --org)", "workflow": "Match workflow name or path (case-insensitive substring)", "label": "Match a job runner label (case-insensitive exact)", "runner-type": "self-hosted or unknown (other labels do not prove hosting type)", "status": "Filter run or job status, depending on view", "since": "Recent failure window (Go duration, e.g. 24h or 168h)", "timeout": "Overall API collection deadline"}
	for name, p := range stringsFlags {
		env := "GHAQUEUE_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
		if v, ok := os.LookupEnv(env); ok {
			*p = v
		}
		f.StringVar(p, name, *p, descriptions[name]+" ("+env+")")
	}
	if v, ok := os.LookupEnv("GHAQUEUE_WORKERS"); ok {
		c.Workers, err = strconv.Atoi(v)
		if err != nil {
			return c, "", false, errors.New("GHAQUEUE_WORKERS must be an integer")
		}
	}
	if v, ok := os.LookupEnv("GHAQUEUE_JSON"); ok {
		c.JSON, err = strconv.ParseBool(v)
		if err != nil {
			return c, "", false, errors.New("GHAQUEUE_JSON must be a boolean")
		}
	}
	for _, setting := range []struct {
		name, env, help string
		value           *bool
	}{
		{"no-color", "GHAQUEUE_NO_COLOR", "Disable color", &c.NoColor},
		{"no-progress", "GHAQUEUE_NO_PROGRESS", "Disable scan progress", &c.NoProgress},
		{"details", "GHAQUEUE_DETAILS", "Show URLs and additional context", &c.Details},
	} {
		if v, ok := os.LookupEnv(setting.env); ok {
			parsed, e := strconv.ParseBool(v)
			if e != nil {
				return c, "", false, fmt.Errorf("%s must be a boolean", setting.env)
			}
			*setting.value = parsed
		}
		f.BoolVar(setting.value, setting.name, *setting.value, setting.help+" ("+setting.env+")")
	}
	f.IntVar(&c.Workers, "workers", c.Workers, "Concurrent repositories (GHAQUEUE_WORKERS)")
	f.BoolVar(&c.JSON, "json", c.JSON, "JSON output (GHAQUEUE_JSON)")
	version := false
	if v, ok := os.LookupEnv("GHAQUEUE_VERSION"); ok {
		version, err = strconv.ParseBool(v)
		if err != nil {
			return c, "", false, err
		}
	}
	f.BoolVar(&version, "version", version, "Print version (GHAQUEUE_VERSION)")
	command := "overview"
	commandSet := false
	rest := []string{}
	// Permit the command before or after flags; values remain attached to their flags.
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			if commandSet || !knownCommand(a) {
				return c, "", false, fmt.Errorf("unknown command %q", a)
			}
			command = a
			commandSet = true
			continue
		}
		rest = append(rest, a)
		name := strings.TrimLeft(strings.SplitN(a, "=", 2)[0], "-")
		if !strings.Contains(a, "=") && name != "json" && name != "version" && name != "help" && name != "h" && name != "no-color" && name != "no-progress" && name != "details" {
			if i+1 < len(args) {
				i++
				rest = append(rest, args[i])
			}
		}
	}
	f.Usage = func() {
		fmt.Fprint(out, "Usage: ghaqueue [overview|jobs|runs|workflows|pools|runners|failures] [flags]\n\nDefault: current repository; use --org to scan an organization.\n")
		f.PrintDefaults()
	}
	if err = f.Parse(rest); err != nil {
		return c, command, false, err
	}
	if version {
		return c, command, true, nil
	}
	if c.Workers < 1 || c.Workers > 64 {
		return c, command, false, errors.New("workers must be between 1 and 64")
	}
	for _, v := range []string{c.Since, c.Timeout} {
		d, e := time.ParseDuration(v)
		if e != nil || d <= 0 {
			return c, command, false, fmt.Errorf("invalid positive duration %q (use 24h or 168h)", v)
		}
	}
	if c.RunnerType != "" && c.RunnerType != "self-hosted" && c.RunnerType != "unknown" {
		return c, command, false, errors.New("runner-type must be self-hosted or unknown; hosted cannot be inferred reliably")
	}
	if c.Status != "" {
		valid := false
		for _, s := range []string{"queued", "in_progress", "waiting", "pending", "requested", "completed"} {
			valid = valid || c.Status == s
		}
		if !valid {
			return c, command, false, errors.New("invalid status")
		}
	}
	explicitScope := false
	orgOverride := os.Getenv("GHAQUEUE_ORG") != ""
	repoOverride := os.Getenv("GHAQUEUE_REPO") != ""
	f.Visit(func(f *flag.Flag) {
		if f.Name == "org" {
			orgOverride = true
		}
		if f.Name == "repo" {
			repoOverride = true
		}
		if f.Name == "org" || f.Name == "repo" {
			explicitScope = true
		}
	})
	if os.Getenv("GHAQUEUE_ORG") != "" || os.Getenv("GHAQUEUE_REPO") != "" {
		explicitScope = true
	}
	if orgOverride && !repoOverride {
		c.Repo = ""
	}
	if !explicitScope {
		if repo := localRepository(); repo != "" {
			c.Repo = repo
			c.Organization = ""
		}
	}
	if c.Repo != "" && !strings.Contains(c.Repo, "/") {
		if c.Organization == "" {
			return c, command, false, errors.New("--repo requires OWNER/REPO or --org")
		}
		c.Repo = c.Organization + "/" + c.Repo
	}
	if c.Repo == "" && c.Organization == "" {
		return c, command, false, errors.New("select --org or --repo, or configure .ghaqueue")
	}
	if c.Repo != "" {
		parts := strings.Split(c.Repo, "/")
		if len(parts) != 2 || !validName(parts[0]) || !validName(parts[1]) {
			return c, command, false, errors.New("repo must be OWNER/REPO")
		}
	}
	if c.Organization != "" && !validName(c.Organization) {
		return c, command, false, errors.New("invalid organization")
	}
	return c, command, false, nil
}
func knownCommand(s string) bool {
	for _, x := range []string{"overview", "jobs", "runs", "workflows", "pools", "runners", "failures"} {
		if s == x {
			return true
		}
	}
	return false
}
func validName(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.", r)) {
			return false
		}
	}
	return true
}
func localRepository() string {
	b, e := exec.Command("git", "remote", "get-url", "origin").Output()
	if e != nil {
		return ""
	}
	return repositoryFromRemote(strings.TrimSpace(string(b)))
}
func repositoryFromRemote(s string) string {
	var p string
	if strings.HasPrefix(s, "git@github.com:") {
		p = strings.TrimPrefix(s, "git@github.com:")
	} else {
		u, e := url.Parse(s)
		if e != nil || u.Hostname() != "github.com" {
			return ""
		}
		p = strings.TrimPrefix(u.Path, "/")
	}
	p = strings.TrimSuffix(p, ".git")
	a := strings.Split(p, "/")
	if len(a) != 2 || !validName(a[0]) || !validName(a[1]) {
		return ""
	}
	return p
}
