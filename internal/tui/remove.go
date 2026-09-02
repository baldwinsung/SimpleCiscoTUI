package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/baldwinsung/SimpleCiscoTUI/internal/cisco"
)

// RemoveAclScreen removes an ACL from an interface.
type RemoveAclScreen struct {
	*tview.Flex
	app  *App
	form *tview.Form
	log  *StatusLog

	ifaceDD       *tview.DropDown
	bindingDD     *tview.DropDown
	ifaceNames    []string
	bindingValues []cisco.Binding
}

func newRemoveAclScreen(app *App) *RemoveAclScreen {
	s := &RemoveAclScreen{app: app}
	s.log = NewStatusLog()

	form := tview.NewForm()
	form.SetItemPadding(0)

	s.ifaceDD = tview.NewDropDown().SetLabel("Interface")
	form.AddFormItem(s.ifaceDD)

	s.bindingDD = tview.NewDropDown().SetLabel("Current bindings")
	form.AddFormItem(s.bindingDD)

	form.AddButton("Remove", func() { s.doRemove() })
	form.AddButton("Back", func() { app.pop() })
	s.form = form

	left := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(form, 6, 0, true)
	left.SetBorderPadding(1, 1, 2, 2)

	body := tview.NewFlex().
		AddItem(left, 48, 0, true).
		AddItem(s.log, 0, 1, false)

	frame := newFrame(body, "Remove ACL", "esc Back")
	s.Flex = tview.NewFlex().AddItem(frame, 0, 1, true)

	loadInterfaces(app, s.log, s.ifaceDD, &s.ifaceNames, func(index int) {
		if index < 0 || index >= len(s.ifaceNames) {
			return
		}
		s.loadBindings(s.ifaceNames[index])
	})

	return s
}

func (s *RemoveAclScreen) loadBindings(iface string) {
	go func() {
		acls, err := s.app.session.InterfaceAcls(iface)
		if err != nil {
			s.app.queueUpdate(func() { s.log.Err(fmt.Sprintf("Failed to read bindings: %s", err)) })
			return
		}
		bindings := acls.Bindings()
		labels := make([]string, len(bindings))
		for i, b := range bindings {
			labels[i] = fmt.Sprintf("%s  (%s)", b.ACL, b.Direction)
		}
		s.app.queueUpdate(func() {
			s.bindingValues = bindings
			s.bindingDD.SetOptions(labels, nil)
			s.bindingDD.SetCurrentOption(-1)
			if len(bindings) > 0 {
				s.log.Info(fmt.Sprintf("%s: %d ACL binding(s).", iface, len(bindings)))
			} else {
				s.log.Info(fmt.Sprintf("%s: no ACLs bound.", iface))
			}
		})
	}()
}

// Shortcut implements the Escape-to-back binding.
func (s *RemoveAclScreen) Shortcut(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyEscape {
		s.app.pop()
		return true
	}
	return false
}

func (s *RemoveAclScreen) doRemove() {
	ifaceIdx, _ := s.ifaceDD.GetCurrentOption()
	bindIdx, _ := s.bindingDD.GetCurrentOption()
	if ifaceIdx < 0 || ifaceIdx >= len(s.ifaceNames) || bindIdx < 0 || bindIdx >= len(s.bindingValues) {
		s.log.Err("Pick an interface and a binding to remove.")
		return
	}
	iface := s.ifaceNames[ifaceIdx]
	binding := s.bindingValues[bindIdx]

	s.setDisabled(true)
	s.log.Info(fmt.Sprintf("Removing %s %s from %s...", binding.ACL, binding.Direction, iface))
	go s.removeWorker(iface, binding.ACL, binding.Direction)
}

func (s *RemoveAclScreen) removeWorker(iface, acl, direction string) {
	out, err := s.app.session.RemoveAcl(iface, acl, direction)
	if err != nil {
		s.app.queueUpdate(func() { s.done(false, fmt.Sprintf("Remove failed: %s", err), "", iface) })
		return
	}
	s.app.queueUpdate(func() { s.done(true, fmt.Sprintf("Removed %s %s from %s.", acl, direction, iface), out, iface) })
}

func (s *RemoveAclScreen) done(ok bool, message, out, iface string) {
	s.setDisabled(false)
	if out != "" {
		s.log.Device(out)
	}
	if ok {
		s.log.OK(message)
	} else {
		s.log.Err(message)
	}
	if ok {
		s.loadBindings(iface) // refresh the list
	}
}

func (s *RemoveAclScreen) setDisabled(disabled bool) {
	s.form.GetButton(0).SetDisabled(disabled) // "Remove" is button index 0
}
