package app

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

type terminal struct {
	w                                         io.Writer
	width                                     int
	tty                                       bool
	profile                                   termenv.Profile
	heading, dim, pending, running, good, bad lipgloss.Style
}

func terminalFile(w io.Writer) *os.File {
	if checked, ok := w.(*checkedWriter); ok {
		return terminalFile(checked.w)
	}
	f, _ := w.(*os.File)
	return f
}
func terminalProfile(tty bool, c Config) termenv.Profile {
	if tty && !c.NoColor && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" {
		return termenv.ANSI256
	}
	return termenv.Ascii
}
func newTerminal(w io.Writer, c Config) *terminal {
	t := &terminal{w: w, width: 100, profile: termenv.Ascii}
	if f := terminalFile(w); f != nil {
		t.tty = term.IsTerminal(int(f.Fd()))
		if width, _, err := term.GetSize(int(f.Fd())); err == nil && width > 0 {
			t.width = width
		}
	}
	renderer := lipgloss.NewRenderer(w)
	t.profile = terminalProfile(t.tty, c)
	renderer.SetColorProfile(t.profile)
	t.heading = renderer.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	t.dim = renderer.NewStyle().Foreground(lipgloss.Color("8"))
	t.pending = renderer.NewStyle().Foreground(lipgloss.Color("3"))
	t.running = renderer.NewStyle().Foreground(lipgloss.Color("6"))
	t.good = renderer.NewStyle().Foreground(lipgloss.Color("2"))
	t.bad = renderer.NewStyle().Foreground(lipgloss.Color("1"))
	return t
}
func (t *terminal) line(s string)    { fmt.Fprintln(t.w, ansi.Wrap(s, max(10, t.width), " ")) }
func (t *terminal) section(s string) { t.line(""); t.line(t.heading.Render(s)) }
func (t *terminal) status(s string) string {
	switch s {
	case "in_progress":
		return t.running.Render("running")
	case "completed", "success", "online":
		return t.good.Render(s)
	case "failure", "timed_out", "action_required", "offline":
		return t.bad.Render(strings.ReplaceAll(s, "_", " "))
	default:
		return t.pending.Render(strings.ReplaceAll(s, "_", " "))
	}
}

// A single renderer owns stderr. Workers report completions to the collector;
// the collector sends phase/counter updates here. No terminal input is read.
type scanProgress struct {
	terminal *terminal
	events   chan scanEvent
	stop     chan struct{}
	finished chan struct{}
	once     sync.Once
}

func startProgress(out, errOut io.Writer, c Config) *scanProgress {
	p := &scanProgress{}
	t := newTerminal(errOut, c)
	if c.JSON || c.NoProgress || !t.tty || !newTerminal(out, c).tty || os.Getenv("TERM") == "dumb" {
		return p
	}
	p.terminal = t
	p.events = make(chan scanEvent, 16)
	p.stop = make(chan struct{})
	p.finished = make(chan struct{})
	go p.run()
	return p
}
func (p *scanProgress) update(e scanEvent) {
	if p.events != nil {
		p.events <- e
	}
}
func (p *scanProgress) finish() {
	if p.stop == nil {
		return
	}
	p.once.Do(func() { close(p.stop); <-p.finished })
}
func (p *scanProgress) run() {
	defer close(p.finished)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	e := scanEvent{Phase: "Connecting to GitHub"}
	started := time.Now()
	draw := func() { fmt.Fprint(p.terminal.w, "\r\x1b[2K", p.terminal.progressLine(e, time.Since(started))) }
	draw()
	for {
		select {
		case next := <-p.events:
			e = next
			draw()
		case <-ticker.C:
			draw()
		case <-p.stop:
			for {
				select {
				case next := <-p.events:
					e = next
				default:
					if e.Phase == "Scanning" && e.Total > 0 {
						draw()
						fmt.Fprintln(p.terminal.w)
					} else {
						fmt.Fprint(p.terminal.w, "\r\x1b[2K")
					}
					return
				}
			}
		}
	}
}
func (t *terminal) progressLine(e scanEvent, elapsed time.Duration) string {
	if e.Phase != "Scanning" || e.Total == 0 {
		return ansi.Truncate("  "+t.heading.Render(e.Phase)+"  "+t.dim.Render(elapsed.Round(time.Second).String()), t.width, "")
	}
	counter := fmt.Sprintf("%*d/%d", len(fmt.Sprint(e.Total)), e.Done, e.Total)
	prefix := "  " + t.heading.Render("Scanning") + "  " + t.dim.Render(counter) + " "
	suffix := fmt.Sprintf(" %3d%%", 100*e.Done/e.Total)
	pctStyle := t.running
	if e.Done*100/e.Total >= 60 {
		pctStyle = t.pending
	}
	if e.Done == e.Total {
		pctStyle = t.good
	}
	width := t.width - lipgloss.Width(prefix) - len(suffix) - 2
	if width < 6 {
		return ansi.Truncate(prefix+suffix, t.width, "")
	}
	bar := progress.New(progress.WithWidth(width), progress.WithSolidFill("2"), progress.WithFillCharacters('█', ' '), progress.WithoutPercentage(), progress.WithColorProfile(t.profile))
	return prefix + t.dim.Render("[") + bar.ViewAs(float64(e.Done)/float64(e.Total)) + t.dim.Render("]") + pctStyle.Render(suffix)
}
