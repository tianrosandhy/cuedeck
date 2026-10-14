package ui

import (
	"bytes"
	"fmt"

	"fyne.io/fyne/v2"

	"cuedeck/internal/audio"
)

const (
	devicePreference    = "output.device"
	defaultDeviceOption = "System default"
)

func RememberedDevice(fyneApp fyne.App) string {
	return fyneApp.Preferences().String(devicePreference)
}

func deviceLabel(device audio.OutputDevice) string {
	label := device.Name
	if device.MaxRate > 0 {
		label = fmt.Sprintf("%s · up to %.1f kHz", device.Name, float64(device.MaxRate)/1000)
	}
	if device.Default {
		label += " (default)"
	}
	return label
}

func (a *App) refreshDevices() {
	devices, err := audio.OutputDevices()
	if err != nil {
		return
	}
	a.applyDevices(devices)
}

func (a *App) applyDevices(devices []audio.OutputDevice) {
	a.deviceLabels, a.deviceIDs = deviceOptions(devices, a.engine.Device())

	selected := defaultDeviceOption
	if id := a.engine.Device().ID; id != "" {
		for i, candidate := range a.deviceIDs {
			if candidate == id {
				selected = a.deviceLabels[i]
				break
			}
		}
	}
	a.deviceSelect.SetOptions(a.deviceLabels)
	a.deviceSelect.SetSelected(selected)
}

func deviceOptions(devices []audio.OutputDevice, current audio.OutputDevice) ([]string, []string) {
	labels := []string{defaultDeviceOption}
	ids := []string{""}
	seen := map[string]int{}
	for _, device := range devices {
		base := deviceLabel(device)
		seen[base]++
		label := base
		if seen[base] > 1 {
			label = fmt.Sprintf("%s (%d)", base, seen[base])
		}
		labels = append(labels, label)
		ids = append(ids, device.ID)
	}
	if current.ID != "" && !hasDevice(devices, current.ID) {
		labels = append(labels, deviceLabel(current)+" (disconnected)")
		ids = append(ids, current.ID)
	}
	return labels, ids
}

func hasDevice(devices []audio.OutputDevice, id string) bool {
	for _, device := range devices {
		if device.ID == id {
			return true
		}
	}
	return false
}

func (a *App) deviceIDFor(label string) string {
	for i, candidate := range a.deviceLabels {
		if candidate == label && i < len(a.deviceIDs) {
			return a.deviceIDs[i]
		}
	}
	return a.engine.Device().ID
}

type transportState struct {
	live     bool
	deviceID string
	position float64
	playing  bool
	volume   float64
	clip     *audio.Clip
	rate     int
}

func (a *App) captureTransport() transportState {
	if a.engine == nil {
		return transportState{}
	}
	return transportState{
		live:     true,
		deviceID: a.engine.Device().ID,
		position: a.engine.Position(),
		playing:  a.engine.IsPlaying(),
		volume:   a.engine.Volume(),
		clip:     a.engine.Clip(),
		rate:     a.engine.Rate(),
	}
}

func (a *App) switchDevice(id string) {
	if a.engine.Device().ID == id {
		return
	}
	previous := a.captureTransport()
	period := a.engine.ConfiguredPeriod()

	effective := id
	replacement, err := audio.StartOn(id, period)
	if err != nil {
		a.fail("Cannot use that output device", err)
		effective = previous.deviceID
		replacement, err = audio.StartOn(effective, period)
		if err != nil {
			a.engine.Close()
			a.engine = nil
			a.fail("Cannot open any output device", err)
			a.refreshDevices()
			return
		}
	}

	old := a.engine
	a.engine = replacement
	if old != nil {
		old.Close()
	}
	if previous.live {
		replacement.SetVolume(previous.volume)
	}
	a.fyneApp.Preferences().SetString(devicePreference, effective)

	switch {
	case previous.clip != nil && previous.rate == replacement.Rate():
		replacement.Load(previous.clip)
		replacement.SeekSeconds(previous.position)
		if previous.playing {
			replacement.Play()
		}
	case len(a.raw) > 0:
		a.redecode(previous.position, previous.playing)
	}

	a.refreshDevices()
}

func (a *App) redecode(position float64, playing bool) {
	if a.engine == nil || len(a.raw) == 0 {
		return
	}
	name := a.proj.Audio.OriginalName
	rate := a.engine.Rate()
	raw := a.raw
	a.showBusy("Re-decoding the audio for the new output rate…")
	go func() {
		clip, err := audio.Decode(name, bytes.NewReader(raw), rate)
		fyne.Do(func() {
			a.hideBusy()
			if err != nil {
				a.fail("Cannot re-decode the audio", err)
				return
			}
			a.engine.Load(clip)
			a.engine.SeekSeconds(position)
			if playing {
				a.engine.Play()
			}
			a.syncTimeline()
		})
	}()
}
