package tui

import (
	"fmt"
	"os"
	"strconv"

	"github.com/rivo/tview"

	"github.com/baldwinsung/SimpleCiscoTUI/internal/cisco"
	"github.com/baldwinsung/SimpleCiscoTUI/internal/config"
)

const (
	lblHost     = "Host"
	lblUsername = "Username"
	lblPassword = "Password"
	lblSecret   = "Enable secret"
	lblPort     = "Port"
)

// ConnectScreen lets the user pick a saved device (from config) or type
// credentials, then connect.
type ConnectScreen struct {
	*tview.Flex // outer, centered container -- what gets pushed as root

	app             *App
	form            *tview.Form
	status          *tview.TextView
	devices         []config.DeviceConfig
	selectedKeyFile string
}

func newConnectScreen(app *App) *ConnectScreen {
	cs := &ConnectScreen{app: app}

	cfg, cfgErr := config.LoadConfig("")
	cs.devices = cfg.Devices

	form := tview.NewForm()
	form.SetButtonsAlign(tview.AlignCenter)
	form.SetItemPadding(0)
	itemCount := 5

	if len(cs.devices) > 0 {
		labels := make([]string, len(cs.devices))
		for i, d := range cs.devices {
			labels[i] = d.Label()
		}
		form.AddDropDown("Saved device", labels, -1, func(_ string, index int) {
			if index < 0 || index >= len(cs.devices) {
				return
			}
			cs.applyDevice(cs.devices[index])
		})
		itemCount++
	}

	form.AddInputField(lblHost, os.Getenv("CISCO_HOST"), 30, nil, nil)
	form.AddInputField(lblUsername, os.Getenv("CISCO_USERNAME"), 30, nil, nil)
	form.AddPasswordField(lblPassword, os.Getenv("CISCO_PASSWORD"), 30, '*', nil)
	form.AddPasswordField(lblSecret, os.Getenv("CISCO_SECRET"), 30, '*', nil)
	port := os.Getenv("CISCO_PORT")
	if port == "" {
		port = "22"
	}
	form.AddInputField(lblPort, port, 10, nil, nil)
	form.AddButton("Connect", cs.doConnect)
	cs.form = form

	status := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter).SetWrap(true)
	cs.status = status

	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, itemCount+4, 0, true).
		AddItem(status, 2, 0, false)
	box.SetBorder(true).
		SetTitle(" Connect to a Cisco device ").
		SetTitleAlign(tview.AlignCenter).
		SetBorderPadding(1, 1, 2, 2)

	cs.Flex = centerFixed(box, 62, itemCount+10)

	if cfgErr != nil {
		cs.setStatus(fmt.Sprintf("[red]Config: %s[-]", cfgErr))
	} else if len(cs.devices) == 1 {
		// Exactly one saved device -> fill it in and connect straight away.
		device := cs.devices[0]
		cs.applyDevice(device)
		cs.setStatus(fmt.Sprintf("[yellow]Connecting to %s...[-]", device.Label()))
		cs.startConnect(device.ToCredentials())
	}

	return cs
}

func (cs *ConnectScreen) setStatus(text string) {
	cs.status.SetText(text)
}

func (cs *ConnectScreen) field(label string) *tview.InputField {
	item := cs.form.GetFormItemByLabel(label)
	if item == nil {
		return nil
	}
	f, _ := item.(*tview.InputField)
	return f
}

func (cs *ConnectScreen) applyDevice(device config.DeviceConfig) {
	username := device.Username
	if username == "" {
		username = device.ToCredentials().Username
	}
	if f := cs.field(lblHost); f != nil {
		f.SetText(device.Host)
	}
	if f := cs.field(lblUsername); f != nil {
		f.SetText(username)
	}
	if f := cs.field(lblPassword); f != nil {
		f.SetText(device.Password)
	}
	if f := cs.field(lblSecret); f != nil {
		f.SetText(device.Secret)
	}
	if f := cs.field(lblPort); f != nil {
		f.SetText(strconv.Itoa(device.Port))
	}
	cs.selectedKeyFile = device.KeyFile
}

func (cs *ConnectScreen) doConnect() {
	host := textOf(cs.field(lblHost))
	username := textOf(cs.field(lblUsername))
	password := textOf(cs.field(lblPassword))
	secret := textOf(cs.field(lblSecret))
	portRaw := textOf(cs.field(lblPort))
	if portRaw == "" {
		portRaw = "22"
	}

	if host == "" || username == "" {
		cs.setStatus("[red]Host and username are required.[-]")
		return
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		cs.setStatus("[red]Port must be a number.[-]")
		return
	}

	cs.setStatus("[yellow]Connecting...[-]")
	cs.startConnect(cisco.Credentials{
		Host:     host,
		Username: username,
		Password: password,
		Secret:   secret,
		Port:     port,
		KeyFile:  cs.selectedKeyFile,
	})
}

func textOf(f *tview.InputField) string {
	if f == nil {
		return ""
	}
	return f.GetText()
}

func (cs *ConnectScreen) startConnect(creds cisco.Credentials) {
	cs.form.GetButton(cs.form.GetButtonCount() - 1).SetDisabled(true)
	go cs.connectWorker(creds)
}

func (cs *ConnectScreen) connectWorker(creds cisco.Credentials) {
	session := cisco.NewSession(creds)
	if err := session.Connect(); err != nil {
		cs.app.queueUpdate(func() { cs.connectFailed(err.Error()) })
		return
	}
	cs.app.queueUpdate(func() { cs.connectOK(session) })
}

func (cs *ConnectScreen) connectOK(session *cisco.Session) {
	cs.app.session = session
	cs.app.push(newMenuScreen(cs.app))
}

func (cs *ConnectScreen) connectFailed(message string) {
	cs.form.GetButton(cs.form.GetButtonCount() - 1).SetDisabled(false)
	cs.setStatus(fmt.Sprintf("[red]%s[-]", tview.Escape(message)))
}
