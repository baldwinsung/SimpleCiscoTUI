package tui

import (
	"fmt"

	"github.com/rivo/tview"
)

// MenuScreen is the main menu: pick one of the three operations.
type MenuScreen struct {
	*tview.Flex
	app *App
	log *StatusLog
}

func newMenuScreen(app *App) *MenuScreen {
	m := &MenuScreen{app: app}

	target := tview.NewTextView().SetDynamicColors(true)
	target.SetText(fmt.Sprintf("[::b]Device:[-::-] %s", app.session.Credentials.Host))
	target.SetBorderPadding(0, 1, 0, 0)

	// A List gives Up/Down navigation and the a/r/s/d accelerator keys for
	// free, matching the original's arrow-key/letter-key menu bindings.
	list := tview.NewList().ShowSecondaryText(false)
	list.AddItem("Apply ACL to interface", "", 'a', m.doApply)
	list.AddItem("Remove ACL from interface", "", 'r', m.doRemove)
	list.AddItem("Copy run -> startup (save)", "", 's', m.doSave)
	list.AddItem("Disconnect", "", 'd', m.doDisconnect)

	actions := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(target, 2, 0, false).
		AddItem(list, 4, 0, true)
	actions.SetBorderPadding(1, 1, 2, 1)

	m.log = NewStatusLog()

	body := tview.NewFlex().
		AddItem(actions, 42, 0, true).
		AddItem(m.log, 0, 1, false)

	footer := "a Apply ACL  r Remove ACL  s Save config  d Disconnect  ctrl+q Quit"
	frame := newFrame(body, appTitle, footer)
	m.Flex = tview.NewFlex().AddItem(frame, 0, 1, true)

	m.log.OK(fmt.Sprintf("Connected to %s", app.session.Credentials.Host))

	return m
}

func (m *MenuScreen) doApply() {
	m.app.push(newApplyAclScreen(m.app))
}

func (m *MenuScreen) doRemove() {
	m.app.push(newRemoveAclScreen(m.app))
}

func (m *MenuScreen) doSave() {
	m.app.push(newSaveScreen(m.app))
}

func (m *MenuScreen) doDisconnect() {
	go func() {
		session := m.app.session
		session.Disconnect()
		m.app.queueUpdate(func() {
			m.app.switchTop(newConnectScreen(m.app))
		})
	}()
}
