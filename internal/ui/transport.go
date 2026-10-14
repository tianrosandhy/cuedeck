package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (a *App) buildTransport() fyne.CanvasObject {
	a.playBtn = widget.NewButtonWithIcon("Play", theme.MediaPlayIcon(), a.togglePlay)
	stopBtn := widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), a.stop)
	barBack := widget.NewButtonWithIcon("", theme.MediaFastRewindIcon(), func() { a.nudgeBars(-1) })
	barFwd := widget.NewButtonWithIcon("", theme.MediaFastForwardIcon(), func() { a.nudgeBars(1) })
	beatBack := widget.NewButton("-1 beat", func() { a.nudgeBeats(-1) })
	beatFwd := widget.NewButton("+1 beat", func() { a.nudgeBeats(1) })
	volume := widget.NewSlider(0, 1)
	volume.Step = 0.02
	volume.Value = 1
	volume.OnChanged = a.engine.SetVolume

	panel := container.NewVBox(
		a.playBtn,
		stopBtn,
		widget.NewSeparator(),
		container.NewGridWithColumns(2, barBack, barFwd),
		container.NewGridWithColumns(2, beatBack, beatFwd),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Master", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWrap(fyne.NewSize(180, 32), volume),
		widget.NewLabelWithStyle("1-9 fire · ↑↓ pick · enter go", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
	)
	return container.NewPadded(container.NewGridWrap(fyne.NewSize(200, panel.MinSize().Height), panel))
}
