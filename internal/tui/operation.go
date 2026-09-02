package tui

import (
	"fmt"

	"github.com/rivo/tview"
)

// directionValues maps the Direction dropdown's option index to the IOS
// direction keyword.
var directionValues = []string{"in", "out"}

var directionLabels = []string{"inbound (in)", "outbound (out)"}

// loadInterfaces runs `show ip interface brief` in the background and
// populates dd with the results, storing the raw interface names (parallel
// to dd's option indices) in *namesOut. onChange, if non-nil, is invoked
// (on the UI goroutine) whenever the user picks a different interface.
func loadInterfaces(app *App, log *StatusLog, dd *tview.DropDown, namesOut *[]string, onChange func(index int)) {
	go func() {
		interfaces, err := app.session.ListInterfaces()
		if err != nil {
			app.queueUpdate(func() { log.Err(fmt.Sprintf("Failed to list interfaces: %s", err)) })
			return
		}
		labels := make([]string, len(interfaces))
		names := make([]string, len(interfaces))
		for i, iface := range interfaces {
			labels[i] = fmt.Sprintf("%s  (%s, %s/%s)", iface.Name, iface.IP, iface.Status, iface.Protocol)
			names[i] = iface.Name
		}
		app.queueUpdate(func() {
			*namesOut = names
			dd.SetOptions(labels, func(_ string, index int) {
				if onChange != nil {
					onChange(index)
				}
			})
			dd.SetCurrentOption(-1)
			log.Info(fmt.Sprintf("Loaded %d interface(s).", len(labels)))
		})
	}()
}
