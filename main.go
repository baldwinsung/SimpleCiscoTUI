// Command simpleciscotui is a tiny terminal UI for interface ACL management
// on Cisco IOS: apply an ACL to an interface, remove one, or copy
// running-config to startup-config.
package main

import (
	"fmt"
	"os"

	"github.com/baldwinsung/SimpleCiscoTUI/internal/tui"
)

func main() {
	if err := tui.NewApp().Run(); err != nil {
		fmt.Fprintln(os.Stderr, "simpleciscotui:", err)
		os.Exit(1)
	}
}
