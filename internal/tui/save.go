package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// SaveScreen confirms and runs "copy running-config startup-config".
type SaveScreen struct {
	*tview.Flex
	app  *App
	form *tview.Form
	log  *StatusLog
}

func newSaveScreen(app *App) *SaveScreen {
	s := &SaveScreen{app: app}
	s.log = NewStatusLog()

	body := tview.NewTextView().
		SetText("This writes the current running configuration to NVRAM so it survives a reload. Continue?").
		SetWrap(true)

	form := tview.NewForm()
	form.SetButtonsAlign(tview.AlignCenter)
	form.SetItemPadding(0)
	form.AddButton("Save", func() { s.doSave() })
	form.AddButton("Cancel", func() { app.pop() })
	s.form = form

	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(body, 2, 0, false).
		AddItem(s.log, 8, 0, false).
		AddItem(form, 3, 0, true)
	box.SetBorder(true).
		SetTitle(" Copy running-config -> startup-config ").
		SetTitleAlign(tview.AlignCenter).
		SetBorderPadding(1, 1, 2, 2)

	s.Flex = centerFixed(box, 66, 18)

	return s
}

// Shortcut implements the Escape-to-back binding.
func (s *SaveScreen) Shortcut(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyEscape {
		s.app.pop()
		return true
	}
	return false
}

func (s *SaveScreen) doSave() {
	s.form.GetButton(0).SetDisabled(true)
	s.log.Info("Saving...")
	go s.saveWorker()
}

func (s *SaveScreen) saveWorker() {
	out, err := s.app.session.SaveConfig()
	if err != nil {
		s.app.queueUpdate(func() { s.done(false, fmt.Sprintf("Save failed: %s", err), "") })
		return
	}
	s.app.queueUpdate(func() { s.done(true, "Saved to startup-config.", out) })
}

func (s *SaveScreen) done(ok bool, message, out string) {
	if out != "" {
		s.log.Device(out)
	}
	if ok {
		s.log.OK(message)
	} else {
		s.log.Err(message)
	}
	s.form.GetButton(0).SetDisabled(false)
}
