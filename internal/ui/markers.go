package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"cuedeck/internal/model"
)

const cueBarWidth = 4

type cueRow struct {
	widget.BaseWidget

	bg      *canvas.Rectangle
	bar     *canvas.Rectangle
	content fyne.CanvasObject
	timing  *widget.Label

	onSelect func()

	playing  bool
	selected bool
	hovered  bool
}

func newCueRow(content fyne.CanvasObject, timing *widget.Label, onSelect func()) *cueRow {
	row := &cueRow{content: content, timing: timing, onSelect: onSelect}
	row.bg = canvas.NewRectangle(colorRowIdle)
	row.bg.CornerRadius = 4
	row.bar = canvas.NewRectangle(colorBarIdle)
	row.ExtendBaseWidget(row)
	return row
}

func (r *cueRow) CreateRenderer() fyne.WidgetRenderer {
	return &cueRowRenderer{row: r, objects: []fyne.CanvasObject{r.bg, r.bar, r.content}}
}

func (r *cueRow) Tapped(*fyne.PointEvent) {
	if r.onSelect != nil {
		r.onSelect()
	}
}

func (r *cueRow) MouseIn(*desktop.MouseEvent) { r.setHovered(true) }

func (r *cueRow) MouseMoved(*desktop.MouseEvent) {}

func (r *cueRow) MouseOut() { r.setHovered(false) }

func (r *cueRow) setPlaying(on bool) {
	if r.playing == on {
		return
	}
	r.playing = on
	r.paint()
}

func (r *cueRow) setSelected(on bool) {
	if r.selected == on {
		return
	}
	r.selected = on
	r.paint()
}

func (r *cueRow) setHovered(on bool) {
	if r.hovered == on {
		return
	}
	r.hovered = on
	r.paint()
}

func (r *cueRow) paint() {
	switch {
	case r.selected:
		r.bg.FillColor = colorRowPicked
	case r.playing:
		r.bg.FillColor = colorRowPlaying
	case r.hovered:
		r.bg.FillColor = colorRowHover
	default:
		r.bg.FillColor = colorRowIdle
	}
	if r.playing {
		r.bar.FillColor = colorBarPlaying
	} else {
		r.bar.FillColor = colorBarIdle
	}
	r.bg.Refresh()
	r.bar.Refresh()
}

type cueRowRenderer struct {
	row     *cueRow
	objects []fyne.CanvasObject
}

func (r *cueRowRenderer) Layout(size fyne.Size) {
	pad := theme.Padding()
	r.row.bg.Resize(size)
	r.row.bg.Move(fyne.NewPos(0, 0))
	r.row.bar.Resize(fyne.NewSize(cueBarWidth, size.Height))
	r.row.bar.Move(fyne.NewPos(0, 0))
	r.row.content.Resize(fyne.NewSize(size.Width-cueBarWidth-pad*2, size.Height-pad))
	r.row.content.Move(fyne.NewPos(cueBarWidth+pad, pad/2))
}

func (r *cueRowRenderer) MinSize() fyne.Size {
	pad := theme.Padding()
	inner := r.row.content.MinSize()
	return fyne.NewSize(inner.Width+cueBarWidth+pad*2, inner.Height+pad)
}

func (r *cueRowRenderer) Refresh() {
	r.row.paint()
}

func (r *cueRowRenderer) Objects() []fyne.CanvasObject { return r.objects }

func (r *cueRowRenderer) Destroy() {}

func (r *cueRow) setTiming(tempo model.Tempo, beat float64) {
	r.timing.SetText(fmt.Sprintf("beat %-10s  %s", formatBeat(beat), formatClock(tempo.BeatToSeconds(beat))))
}

func (a *App) buildCuePanel() fyne.CanvasObject {
	a.cueList = container.NewVBox()
	a.cueTitle = widget.NewLabelWithStyle("Cues", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	hint := widget.NewLabelWithStyle("↑ ↓ pick the next part, enter plays it · 1-9 fire instantly", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	hint.SizeName = theme.SizeNameCaptionText
	header := container.NewPadded(container.NewVBox(a.cueTitle, hint))
	a.cueScroll = container.NewVScroll(a.cueList)
	return container.NewBorder(header, nil, nil, nil, a.cueScroll)
}

func (a *App) buildRows() {
	objects := make([]fyne.CanvasObject, 0, len(a.proj.Markers))
	rows := make([]*cueRow, 0, len(a.proj.Markers))
	for i, marker := range a.proj.Markers {
		row := a.newCueRow(i, marker)
		rows = append(rows, row)
		objects = append(objects, row)
	}
	a.rows = rows
	a.cueList.Objects = objects
	a.cueList.Refresh()

	if a.selected >= len(a.proj.Markers) {
		a.selected = len(a.proj.Markers) - 1
	}

	count := fmt.Sprintf("Cues (%d)", len(a.proj.Markers))
	if a.cueTitle.Text != count {
		a.cueTitle.SetText(count)
	}
	a.highlightRows()
	a.playhead.SetSelected(a.selected)
}

func (a *App) newCueRow(index int, marker model.Marker) *cueRow {
	hotkey := "·"
	if index < 9 {
		hotkey = strconv.Itoa(index + 1)
	}
	hotkeyLabel := widget.NewLabelWithStyle(hotkey, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	nameLabel := widget.NewLabelWithStyle(marker.Label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	timing := widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{Monospace: true})

	left := container.NewHBox(container.NewGridWrap(fyne.NewSize(30, 28), hotkeyLabel), nameLabel)
	right := container.NewHBox(
		widget.NewButtonWithIcon("Fire", theme.MediaPlayIcon(), func() { a.trigger(index) }),
		widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() { a.editCue(index) }),
		widget.NewButtonWithIcon("", theme.ContentClearIcon(), func() { a.deleteCue(index) }),
	)
	content := container.NewBorder(nil, nil, left, right, timing)

	row := newCueRow(content, timing, func() { a.selectCue(index) })
	row.setTiming(a.proj.Tempo, marker.Beat)
	return row
}

func (a *App) highlightRows() {
	for i, row := range a.rows {
		row.setPlaying(i == a.section)
		row.setSelected(i == a.selected)
	}
}

func (a *App) selectCue(index int) {
	if index < 0 || index >= len(a.proj.Markers) {
		return
	}
	if index == a.selected {
		return
	}
	a.selected = index
	a.highlightRows()
	a.playhead.SetSelected(index)
	a.scrollToSelection()
}

func (a *App) moveSelection(delta int) {
	count := len(a.proj.Markers)
	if count == 0 {
		return
	}
	next := a.selected + delta
	if a.selected < 0 {
		base := a.section
		if base < 0 {
			base = -1
		}
		next = base + delta
	}
	if next < 0 {
		next = 0
	}
	if next > count-1 {
		next = count - 1
	}
	a.selectCue(next)
}

func (a *App) playSelection() {
	index := a.selected
	if index < 0 || index >= len(a.proj.Markers) {
		index = a.section
	}
	if index < 0 || index >= len(a.proj.Markers) {
		return
	}
	a.selected = index
	a.jumpTo(index)
	if !a.engine.IsPlaying() {
		a.engine.Play()
	}
	a.scrollToSelection()
}

func (a *App) scrollToSelection() {
	if a.cueScroll == nil || a.selected < 0 || a.selected >= len(a.rows) {
		return
	}
	a.cueScroll.Refresh()
	view := a.cueScroll.Size().Height
	if view <= 0 {
		return
	}
	row := a.rows[a.selected]
	top := row.Position().Y
	bottom := top + row.Size().Height
	offset := a.cueScroll.Offset.Y
	switch {
	case top < offset:
		a.cueScroll.ScrollToOffset(fyne.NewPos(0, top))
	case bottom > offset+view:
		a.cueScroll.ScrollToOffset(fyne.NewPos(0, bottom-view))
	}
}

func (a *App) snapBeat(beat float64) float64 {
	if a.snap == nil || !a.snap.Checked {
		return beat
	}
	return model.Snap(beat, 0.25)
}

func (a *App) addCueAtPlayhead() {
	if !a.engine.HasClip() {
		return
	}
	beat := a.snapBeat(a.proj.Tempo.SecondsToBeat(a.engine.Position()))
	a.proj.Add(a.proj.AutoLabel(), beat)
	a.buildRows()
	a.syncTimeline()
	a.markDirty()
}

func (a *App) addCueAtBeat() {
	beat := a.proj.Tempo.SecondsToBeat(a.engine.Position())
	preview := widget.NewLabel("")
	nameEntry := widget.NewEntry()
	nameEntry.SetText(a.proj.AutoLabel())
	beatEntry := widget.NewEntry()
	beatEntry.SetText(formatBeat(beat))
	beatEntry.TextStyle = fyne.TextStyle{Monospace: true}

	update := func() {
		seconds := a.proj.Tempo.BeatToSeconds(parseFloat(beatEntry.Text, beat))
		preview.SetText(fmt.Sprintf("%s  ·  bar %s", formatClock(seconds), a.proj.Tempo.FormatBeat(parseFloat(beatEntry.Text, beat))))
	}
	beatEntry.OnChanged = func(string) { update() }
	update()

	content := container.NewVBox(
		widget.NewLabelWithStyle("New cue", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(2, widget.NewLabel("Label"), nameEntry),
		container.NewGridWithColumns(2, widget.NewLabel("Beat"), beatEntry),
		container.NewGridWithColumns(2, widget.NewLabel("Time"), preview),
	)
	d := dialog.NewCustomConfirm("Add cue", "Add", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		a.proj.Add(strings.TrimSpace(nameEntry.Text), parseFloat(beatEntry.Text, beat))
		a.buildRows()
		a.syncTimeline()
		a.markDirty()
	}, a.win)
	d.Resize(fyne.NewSize(460, 300))
	d.Show()
}

func (a *App) editCue(index int) {
	if index < 0 || index >= len(a.proj.Markers) {
		return
	}
	marker := a.proj.Markers[index]
	preview := widget.NewLabel("")
	nameEntry := widget.NewEntry()
	nameEntry.SetText(marker.Label)
	beatEntry := widget.NewEntry()
	beatEntry.SetText(formatBeat(marker.Beat))
	beatEntry.TextStyle = fyne.TextStyle{Monospace: true}

	update := func() {
		beat := parseFloat(beatEntry.Text, marker.Beat)
		preview.SetText(fmt.Sprintf("%s  ·  bar %s", formatClock(a.proj.Tempo.BeatToSeconds(beat)), a.proj.Tempo.FormatBeat(beat)))
	}
	beatEntry.OnChanged = func(string) { update() }
	update()

	fire := widget.NewButtonWithIcon("Play from here", theme.MediaPlayIcon(), func() {
		a.jumpTo(index)
		if !a.engine.IsPlaying() {
			a.engine.Play()
		}
	})
	round := widget.NewButton("Snap to nearest beat", func() {
		beatEntry.SetText(formatBeat(model.Snap(parseFloat(beatEntry.Text, marker.Beat), 1)))
		update()
	})

	content := container.NewVBox(
		widget.NewLabelWithStyle(marker.Label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(2, widget.NewLabel("Label"), nameEntry),
		container.NewGridWithColumns(2, widget.NewLabel("Beat"), beatEntry),
		container.NewGridWithColumns(2, widget.NewLabel("Time"), preview),
		container.NewHBox(fire, round),
	)
	d := dialog.NewCustomConfirm("Edit cue", "Save", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		a.proj.Update(marker.ID, strings.TrimSpace(nameEntry.Text), parseFloat(beatEntry.Text, marker.Beat))
		a.buildRows()
		a.syncTimeline()
		a.markDirty()
	}, a.win)
	d.Resize(fyne.NewSize(500, 340))
	d.Show()
}

func (a *App) deleteCue(index int) {
	if index < 0 || index >= len(a.proj.Markers) {
		return
	}
	marker := a.proj.Markers[index]
	dialog.ShowConfirm("Delete cue", fmt.Sprintf("Delete “%s”?", marker.Label), func(ok bool) {
		if !ok {
			return
		}
		a.proj.Remove(marker.ID)
		a.buildRows()
		a.syncTimeline()
		a.markDirty()
	}, a.win)
}
