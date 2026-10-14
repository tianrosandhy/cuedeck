package ui

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed icon.png
var iconPNG []byte

func appIcon() fyne.Resource {
	return fyne.NewStaticResource("icon.png", iconPNG)
}
