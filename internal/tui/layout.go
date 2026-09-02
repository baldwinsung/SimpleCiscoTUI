package tui

import "github.com/rivo/tview"

// centerFixed centers p in a fixed-size box within the full terminal --
// the equivalent of Textual's <Middle><Center>...</Center></Middle>.
func centerFixed(p tview.Primitive, width, height int) *tview.Flex {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(
			tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(nil, 0, 1, false).
				AddItem(p, height, 0, true).
				AddItem(nil, 0, 1, false),
			width, 0, true,
		).
		AddItem(nil, 0, 1, false)
}
