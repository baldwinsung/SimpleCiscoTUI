// Package tui is the SimpleCiscoTUI terminal UI, built on tview.
//
// Screen flow mirrors the original: ConnectScreen -> MenuScreen ->
// {ApplyAclScreen, RemoveAclScreen, SaveScreen}. Every device interaction
// runs in a goroutine (the analogue of a Textual thread worker) and
// marshals UI updates back with App.queueUpdate (tapp.QueueUpdateDraw),
// mirroring call_from_thread.
package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/baldwinsung/SimpleCiscoTUI/internal/cisco"
)

const appTitle = "SimpleCiscoTUI"

// Shortcuts is implemented by screens that want first crack at a key press
// while they are the active (topmost) screen -- e.g. Escape to go back, or
// the Menu screen's a/r/s/d action letters.
type Shortcuts interface {
	Shortcut(event *tcell.EventKey) bool
}

// App owns the tview application, the screen stack, and the current device
// session.
type App struct {
	tapp    *tview.Application
	session *cisco.Session
	stack   []tview.Primitive
}

// NewApp constructs the application and wires global key handling.
func NewApp() *App {
	a := &App{tapp: tview.NewApplication()}
	a.tapp.SetInputCapture(a.globalInputCapture)
	return a
}

// Run pushes the initial Connect screen and starts the event loop.
func (a *App) Run() error {
	a.push(newConnectScreen(a))
	err := a.tapp.Run()
	if a.session != nil {
		a.session.Disconnect()
	}
	return err
}

func (a *App) top() tview.Primitive {
	if len(a.stack) == 0 {
		return nil
	}
	return a.stack[len(a.stack)-1]
}

// push shows a new screen on top of the stack (Screen.push_screen equivalent).
func (a *App) push(p tview.Primitive) {
	a.stack = append(a.stack, p)
	a.tapp.SetRoot(p, true)
	a.tapp.SetFocus(p)
}

// pop discards the topmost screen and shows the one beneath it
// (app.pop_screen equivalent).
func (a *App) pop() {
	if len(a.stack) < 2 {
		return
	}
	a.stack = a.stack[:len(a.stack)-1]
	top := a.top()
	a.tapp.SetRoot(top, true)
	a.tapp.SetFocus(top)
}

// switchTop replaces the current screen in place (app.switch_screen
// equivalent, used when disconnecting back to the Connect screen).
func (a *App) switchTop(p tview.Primitive) {
	if len(a.stack) == 0 {
		a.stack = []tview.Primitive{p}
	} else {
		a.stack[len(a.stack)-1] = p
	}
	a.tapp.SetRoot(p, true)
	a.tapp.SetFocus(p)
}

// queueUpdate marshals a UI update back onto the event loop goroutine
// (self.app.call_from_thread equivalent). Call it from any goroutine that
// touches widgets.
func (a *App) queueUpdate(f func()) {
	a.tapp.QueueUpdateDraw(f)
}

// globalInputCapture handles bindings that apply everywhere: Ctrl+Q to
// quit, Down/Up promoted to Tab/Backtab so arrow keys move focus like the
// original (except while a DropDown's option list needs them), and the
// active screen's own Shortcut hook (Escape-to-back, Menu's a/r/s/d).
func (a *App) globalInputCapture(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyCtrlQ {
		a.tapp.Stop()
		return nil
	}

	if top := a.top(); top != nil {
		if sc, ok := top.(Shortcuts); ok {
			if sc.Shortcut(event) {
				return nil
			}
		}
	}

	// An expanded DropDown hands focus to its internal option List while
	// open (and the Menu screen's own List needs Up/Down natively too);
	// everywhere else, arrow keys move focus like Tab/Shift+Tab.
	switch a.tapp.GetFocus().(type) {
	case *tview.DropDown, *tview.List:
		return event
	}
	switch event.Key() {
	case tcell.KeyDown:
		return tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
	case tcell.KeyUp:
		return tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)
	}
	return event
}

// newFrame wraps content in a titled header / hint footer, the equivalent
// of Textual's automatic Header()/Footer().
func newFrame(content tview.Primitive, title, footer string) *tview.Frame {
	frame := tview.NewFrame(content).
		SetBorders(0, 0, 1, 1, 2, 2)
	frame.AddText(title, true, tview.AlignLeft, tcell.ColorAqua)
	if footer != "" {
		frame.AddText(footer, false, tview.AlignLeft, tcell.ColorGray)
	}
	return frame
}
