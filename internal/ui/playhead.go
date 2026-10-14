package ui

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

var (
	colorStrip      = color.NRGBA{R: 24, G: 26, B: 32, A: 255}
	colorPlayed     = color.NRGBA{R: 40, G: 96, B: 170, A: 170}
	colorRegionA    = color.NRGBA{R: 255, G: 255, B: 255, A: 6}
	colorRegionB    = color.NRGBA{R: 255, G: 255, B: 255, A: 26}
	colorRegionPick = color.NRGBA{R: 255, G: 198, B: 92, A: 55}
	colorTick       = color.NRGBA{R: 255, G: 190, B: 80, A: 220}
	colorTickLive   = color.NRGBA{R: 255, G: 236, B: 160, A: 255}
	colorTickPick   = color.NRGBA{R: 255, G: 226, B: 140, A: 255}
	colorCursor     = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	colorRowIdle    = color.NRGBA{R: 30, G: 33, B: 40, A: 255}
	colorRowHover   = color.NRGBA{R: 42, G: 46, B: 57, A: 255}
	colorRowPlaying = color.NRGBA{R: 34, G: 52, B: 84, A: 255}
	colorRowPicked  = color.NRGBA{R: 98, G: 76, B: 28, A: 255}
	colorBarPlaying = color.NRGBA{R: 90, G: 170, B: 255, A: 255}
	colorBarIdle    = color.NRGBA{R: 0, G: 0, B: 0, A: 0}
	colorAccent     = color.NRGBA{R: 255, G: 200, B: 96, A: 255}
	colorDim        = color.NRGBA{R: 150, G: 156, B: 168, A: 255}
)

type valueText struct {
	widget.BaseWidget
	text *canvas.Text
}

func newValueText(size float32, mono bool) *valueText {
	v := &valueText{text: canvas.NewText("", color.White)}
	v.text.TextSize = size
	v.text.TextStyle = fyne.TextStyle{Monospace: mono}
	v.ExtendBaseWidget(v)
	return v
}

func (v *valueText) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(v.text)
}

func (v *valueText) SetText(s string) {
	if v.text.Text == s {
		return
	}
	v.text.Text = s
	v.text.Refresh()
}

func (v *valueText) SetColor(c color.Color) {
	v.text.Color = c
	v.text.Refresh()
}

type playhead struct {
	widget.BaseWidget

	duration float64
	position float64
	marks    []float64
	section  int
	selected int
	dirty    bool

	onScrub func(seconds float64)
	onStep  func(delta float64)
}

func newPlayhead(onScrub func(float64), onStep func(float64)) *playhead {
	p := &playhead{onScrub: onScrub, onStep: onStep, section: -1, selected: -1, dirty: true}
	p.ExtendBaseWidget(p)
	return p
}

func (p *playhead) CreateRenderer() fyne.WidgetRenderer {
	r := &playheadRenderer{playhead: p}
	r.build()
	return r
}

func (p *playhead) MinSize() fyne.Size {
	return fyne.NewSize(320, 44)
}

func (p *playhead) SetDuration(seconds float64) {
	if p.duration == seconds {
		return
	}
	p.duration = seconds
	p.Refresh()
}

func (p *playhead) SetPosition(seconds float64) {
	if p.position == seconds {
		return
	}
	p.position = seconds
	p.Refresh()
}

func (p *playhead) SetSection(index int) {
	if p.section == index {
		return
	}
	p.section = index
	p.Refresh()
}

func (p *playhead) SetSelected(index int) {
	if p.selected == index {
		return
	}
	p.selected = index
	p.Refresh()
}

func (p *playhead) SetMarks(marks []float64) {
	if sameFloats(p.marks, marks) {
		return
	}
	p.marks = append(p.marks[:0], marks...)
	p.dirty = true
	p.Refresh()
}

func (p *playhead) Tapped(ev *fyne.PointEvent) {
	p.scrubAt(ev.Position.X)
}

func (p *playhead) Dragged(ev *fyne.DragEvent) {
	p.scrubAt(ev.Position.X)
}

func (p *playhead) DragEnd() {}

func (p *playhead) Scrolled(ev *fyne.ScrollEvent) {
	if p.onStep != nil {
		p.onStep(float64(ev.Scrolled.DY))
	}
}

func (p *playhead) scrubAt(x float32) {
	if p.duration <= 0 || p.onScrub == nil {
		return
	}
	width := p.Size().Width
	if width <= 0 {
		return
	}
	frac := float64(x / width)
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	p.onScrub(frac * p.duration)
}

type playheadRenderer struct {
	playhead *playhead
	bg       *canvas.Rectangle
	played   *canvas.Rectangle
	cursor   *canvas.Line
	hint     *canvas.Text
	regions  []*canvas.Rectangle
	ticks    []*canvas.Line
	objects  []fyne.CanvasObject
}

func (r *playheadRenderer) build() {
	r.bg = canvas.NewRectangle(colorStrip)
	r.bg.CornerRadius = 6
	r.played = canvas.NewRectangle(colorPlayed)
	r.played.CornerRadius = 6
	r.cursor = canvas.NewLine(colorCursor)
	r.cursor.StrokeWidth = 2
	r.hint = canvas.NewText("open an audio file to see the cue map", colorDim)
	r.hint.TextSize = 13
	r.hint.Alignment = fyne.TextAlignCenter
	r.regions = nil
	r.ticks = nil
	r.rebuild()
}

func (r *playheadRenderer) rebuild() {
	p := r.playhead
	r.regions = make([]*canvas.Rectangle, 0, len(p.marks))
	r.ticks = make([]*canvas.Line, 0, len(p.marks))
	objects := make([]fyne.CanvasObject, 0, len(p.marks)*2+3)
	objects = append(objects, r.bg, r.played)
	for range p.marks {
		region := canvas.NewRectangle(colorRegionA)
		region.CornerRadius = 0
		r.regions = append(r.regions, region)
		objects = append(objects, region)
	}
	for range p.marks {
		tick := canvas.NewLine(colorTick)
		tick.StrokeWidth = 2
		r.ticks = append(r.ticks, tick)
		objects = append(objects, tick)
	}
	objects = append(objects, r.cursor, r.hint)
	r.objects = objects
	p.dirty = false
}

func (r *playheadRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.bg.Move(fyne.NewPos(0, 0))

	p := r.playhead
	at := func(seconds float64) float32 {
		if p.duration <= 0 {
			return 0
		}
		frac := seconds / p.duration
		if frac < 0 {
			frac = 0
		}
		if frac > 1 {
			frac = 1
		}
		return float32(frac) * size.Width
	}

	played := at(p.position)
	r.played.Resize(fyne.NewSize(played, size.Height))
	r.played.Move(fyne.NewPos(0, 0))

	for i, region := range r.regions {
		start := at(p.marks[i])
		end := size.Width
		if i+1 < len(p.marks) {
			end = at(p.marks[i+1])
		}
		if i == p.selected {
			region.FillColor = colorRegionPick
		} else if i%2 == 1 {
			region.FillColor = colorRegionB
		} else {
			region.FillColor = colorRegionA
		}
		region.Move(fyne.NewPos(start, 0))
		region.Resize(fyne.NewSize(end-start, size.Height))
	}

	for i, tick := range r.ticks {
		x := at(p.marks[i])
		tick.Position1 = fyne.NewPos(x, 0)
		tick.Position2 = fyne.NewPos(x, size.Height)
		switch {
		case i == p.selected:
			tick.StrokeColor = colorTickPick
			tick.StrokeWidth = 4
		case i == p.section:
			tick.StrokeColor = colorTickLive
			tick.StrokeWidth = 3
		default:
			tick.StrokeColor = colorTick
			tick.StrokeWidth = 2
		}
	}

	r.cursor.Position1 = fyne.NewPos(played, 0)
	r.cursor.Position2 = fyne.NewPos(played, size.Height)

	if p.duration <= 0 {
		r.hint.Resize(size)
		r.hint.Move(fyne.NewPos(0, size.Height/2-10))
		r.hint.Show()
	} else {
		r.hint.Hide()
	}
}

func (r *playheadRenderer) MinSize() fyne.Size {
	return r.playhead.MinSize()
}

func (r *playheadRenderer) Refresh() {
	if r.playhead.dirty {
		r.rebuild()
	}
	r.Layout(r.playhead.Size())
}

func (r *playheadRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *playheadRenderer) Destroy() {}

func sameFloats(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-9 {
			return false
		}
	}
	return true
}
