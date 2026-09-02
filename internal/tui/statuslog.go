package tui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

// StatusLog is the shared output pane. Color-coded helpers mirror the
// Python original's ok/err/info/device RichLog helpers.
type StatusLog struct {
	*tview.TextView
}

// NewStatusLog creates an empty, auto-scrolling status log.
func NewStatusLog() *StatusLog {
	tv := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetScrollable(true)
	tv.SetBorderPadding(1, 1, 2, 2)
	return &StatusLog{tv}
}

func (s *StatusLog) writeLine(line string) {
	fmt.Fprintln(s, line)
	s.ScrollToEnd()
}

// OK writes a green checkmark line.
func (s *StatusLog) OK(msg string) {
	s.writeLine(fmt.Sprintf("[green]✓[-] %s", tview.Escape(msg)))
}

// Err writes a red cross line.
func (s *StatusLog) Err(msg string) {
	s.writeLine(fmt.Sprintf("[red]✗[-] %s", tview.Escape(msg)))
}

// Info writes a dim informational line.
func (s *StatusLog) Info(msg string) {
	s.writeLine(fmt.Sprintf("[gray]·[-] %s", tview.Escape(msg)))
}

// Device writes raw device output, if any, in cyan.
func (s *StatusLog) Device(msg string) {
	text := strings.TrimSpace(msg)
	if text != "" {
		s.writeLine(fmt.Sprintf("[aqua]%s[-]", tview.Escape(text)))
	}
}
