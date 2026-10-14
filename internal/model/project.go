package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"time"
)

const TicksPerBeat = 960

type Tempo struct {
	BPM      float64 `json:"bpm"`
	BeatZero float64 `json:"beatZero"`
	SigNum   int     `json:"sigNum"`
	SigDen   int     `json:"sigDen"`
}

func DefaultTempo() Tempo {
	return Tempo{BPM: 120, SigNum: 4, SigDen: 4}
}

func (t Tempo) norm() Tempo {
	if !(t.BPM > 0) || math.IsInf(t.BPM, 0) {
		t.BPM = 120
	}
	if t.SigNum <= 0 {
		t.SigNum = 4
	}
	if t.SigDen <= 0 {
		t.SigDen = 4
	}
	if math.IsNaN(t.BeatZero) || math.IsInf(t.BeatZero, 0) {
		t.BeatZero = 0
	}
	return t
}

func (t Tempo) SecondsPerBeat() float64 {
	return 60 / t.norm().BPM
}

func (t Tempo) BeatsPerBar() float64 {
	n := t.norm()
	return float64(n.SigNum) * 4 / float64(n.SigDen)
}

func (t Tempo) BeatToSeconds(beat float64) float64 {
	return t.norm().BeatZero + beat*t.SecondsPerBeat()
}

func (t Tempo) SecondsToBeat(sec float64) float64 {
	return (sec - t.norm().BeatZero) / t.SecondsPerBeat()
}

func (t Tempo) BarBeatTick(beat float64) (int, int, int) {
	if math.IsNaN(beat) || math.IsInf(beat, 0) {
		beat = 0
	}
	bpb := t.BeatsPerBar()
	bar := math.Floor(beat / bpb)
	rest := beat - bar*bpb
	whole := math.Floor(rest)
	tick := int(math.Round((rest - whole) * TicksPerBeat))
	if tick >= TicksPerBeat {
		tick = 0
		whole++
	}
	if whole >= bpb {
		whole = 0
		bar++
	}
	return int(bar) + 1, int(whole) + 1, tick
}

func (t Tempo) FormatBeat(beat float64) string {
	if beat < 0 {
		return "pre-roll"
	}
	bar, inBar, tick := t.BarBeatTick(beat)
	return fmt.Sprintf("%d.%d.%03d", bar, inBar, tick)
}

func Snap(beat, grid float64) float64 {
	if grid <= 0 || math.IsNaN(beat) || math.IsInf(beat, 0) {
		return beat
	}
	return math.Round(beat/grid) * grid
}

type Marker struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	Beat  float64 `json:"beat"`
}

func NewMarker(label string, beat float64) Marker {
	if label == "" {
		label = "Marker"
	}
	return Marker{ID: newID(), Label: label, Beat: beat}
}

func newID() string {
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("m%d", time.Now().UnixNano())
	}
	return "m" + hex.EncodeToString(b[:])
}

type AudioInfo struct {
	OriginalName string `json:"originalName"`
	Format       string `json:"format"`
	SourceRate   int    `json:"sourceRate"`
	Bytes        int64  `json:"bytes"`
}

type Project struct {
	Name    string    `json:"name"`
	Tempo   Tempo     `json:"tempo"`
	Markers []Marker  `json:"markers"`
	Audio   AudioInfo `json:"audio"`
}

func NewProject() *Project {
	return &Project{Name: "Untitled", Tempo: DefaultTempo()}
}

func (p *Project) Sort() {
	sort.SliceStable(p.Markers, func(i, j int) bool {
		if p.Markers[i].Beat == p.Markers[j].Beat {
			return p.Markers[i].ID < p.Markers[j].ID
		}
		return p.Markers[i].Beat < p.Markers[j].Beat
	})
}

func (p *Project) Add(label string, beat float64) int {
	marker := NewMarker(label, beat)
	p.Markers = append(p.Markers, marker)
	p.Sort()
	return p.Index(marker.ID)
}

func (p *Project) Index(id string) int {
	for i := range p.Markers {
		if p.Markers[i].ID == id {
			return i
		}
	}
	return -1
}

func (p *Project) Remove(id string) bool {
	i := p.Index(id)
	if i < 0 {
		return false
	}
	p.Markers = append(p.Markers[:i], p.Markers[i+1:]...)
	return true
}

func (p *Project) Update(id, label string, beat float64) bool {
	i := p.Index(id)
	if i < 0 {
		return false
	}
	p.Markers[i].Label = label
	p.Markers[i].Beat = beat
	p.Sort()
	return true
}

func (p *Project) SectionAt(beat float64) int {
	section := -1
	for i := range p.Markers {
		if p.Markers[i].Beat <= beat+1e-9 {
			section = i
		}
	}
	return section
}

func (p *Project) AutoLabel() string {
	used := make(map[string]bool, len(p.Markers))
	for _, m := range p.Markers {
		used[m.Label] = true
	}
	for i := 1; ; i++ {
		label := fmt.Sprintf("M%d", i)
		if !used[label] {
			return label
		}
	}
}
