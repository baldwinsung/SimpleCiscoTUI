package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ApplyAclScreen applies an ACL to an interface.
type ApplyAclScreen struct {
	*tview.Flex
	app  *App
	form *tview.Form
	log  *StatusLog

	ifaceDD     *tview.DropDown
	aclInput    *tview.InputField
	directionDD *tview.DropDown
	ifaceNames  []string
}

func newApplyAclScreen(app *App) *ApplyAclScreen {
	s := &ApplyAclScreen{app: app}
	s.log = NewStatusLog()

	form := tview.NewForm()
	form.SetItemPadding(0)

	s.ifaceDD = tview.NewDropDown().SetLabel("Interface")
	form.AddFormItem(s.ifaceDD)

	form.AddInputField("ACL name", "", 30, nil, nil)
	s.aclInput = form.GetFormItem(form.GetFormItemCount() - 1).(*tview.InputField)

	form.AddDropDown("Direction", directionLabels, 0, nil)
	s.directionDD = form.GetFormItem(form.GetFormItemCount() - 1).(*tview.DropDown)

	form.AddButton("Apply", func() { s.doApply() })
	form.AddButton("Back", func() { app.pop() })
	s.form = form

	left := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(form, 7, 0, true)
	left.SetBorderPadding(1, 1, 2, 2)

	body := tview.NewFlex().
		AddItem(left, 48, 0, true).
		AddItem(s.log, 0, 1, false)

	frame := newFrame(body, "Apply ACL", "esc Back")
	s.Flex = tview.NewFlex().AddItem(frame, 0, 1, true)

	loadInterfaces(app, s.log, s.ifaceDD, &s.ifaceNames, nil)

	return s
}

// Shortcut implements the Escape-to-back binding.
func (s *ApplyAclScreen) Shortcut(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyEscape {
		s.app.pop()
		return true
	}
	return false
}

func (s *ApplyAclScreen) doApply() {
	idx, _ := s.ifaceDD.GetCurrentOption()
	acl := strings.TrimSpace(s.aclInput.GetText())
	if idx < 0 || idx >= len(s.ifaceNames) || acl == "" {
		s.log.Err("Pick an interface and enter an ACL name.")
		return
	}
	iface := s.ifaceNames[idx]
	dirIdx, _ := s.directionDD.GetCurrentOption()
	if dirIdx < 0 || dirIdx >= len(directionValues) {
		dirIdx = 0
	}
	direction := directionValues[dirIdx]

	s.setDisabled(true)
	s.log.Info(fmt.Sprintf("Applying %s %s on %s...", acl, direction, iface))
	go s.applyWorker(iface, acl, direction)
}

func (s *ApplyAclScreen) applyWorker(iface, acl, direction string) {
	out, err := s.app.session.ApplyAcl(iface, acl, direction)
	if err != nil {
		s.app.queueUpdate(func() { s.done(false, fmt.Sprintf("Apply failed: %s", err), "") })
		return
	}
	s.app.queueUpdate(func() { s.done(true, fmt.Sprintf("Applied %s %s on %s.", acl, direction, iface), out) })
}

func (s *ApplyAclScreen) done(ok bool, message, out string) {
	s.setDisabled(false)
	if out != "" {
		s.log.Device(out)
	}
	if ok {
		s.log.OK(message)
	} else {
		s.log.Err(message)
	}
}

func (s *ApplyAclScreen) setDisabled(disabled bool) {
	s.form.GetButton(0).SetDisabled(disabled) // "Apply" is button index 0
}
