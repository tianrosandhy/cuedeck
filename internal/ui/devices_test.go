package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"cuedeck/internal/audio"
)

func TestDeviceOptionsAreBuiltFromTheDeviceList(t *testing.T) {
	a := newTestApp(t)
	a.applyDevices([]audio.OutputDevice{
		{ID: "abc", Name: "MacBook Pro Speakers", MaxRate: 96000, Default: true},
		{ID: "def", Name: "Scarlett 2i2 USB", MaxRate: 192000},
	})

	want := []string{
		defaultDeviceOption,
		"MacBook Pro Speakers · up to 96.0 kHz (default)",
		"Scarlett 2i2 USB · up to 192.0 kHz",
	}
	if len(a.deviceSelect.Options) != len(want) {
		t.Fatalf("the picker offers %v", a.deviceSelect.Options)
	}
	for i, label := range want {
		if a.deviceSelect.Options[i] != label {
			t.Fatalf("option %d reads %q, expected %q", i, a.deviceSelect.Options[i], label)
		}
	}
	if len(a.deviceLabels) != len(a.deviceIDs) {
		t.Fatalf("labels and ids are out of step: %v vs %v", a.deviceLabels, a.deviceIDs)
	}
	if a.deviceSelect.Selected != defaultDeviceOption {
		t.Fatalf("the picker starts on %q", a.deviceSelect.Selected)
	}
}

func TestDeviceLabelsStayUniqueWhenNamesClash(t *testing.T) {
	a := newTestApp(t)
	a.applyDevices([]audio.OutputDevice{
		{ID: "one", Name: "USB Audio", MaxRate: 48000},
		{ID: "two", Name: "USB Audio", MaxRate: 48000},
		{ID: "three", Name: "USB Audio", MaxRate: 48000},
	})

	seen := map[string]bool{}
	for _, label := range a.deviceSelect.Options {
		if label == defaultDeviceOption {
			continue
		}
		if seen[label] {
			t.Fatalf("option %q is offered twice: %v", label, a.deviceSelect.Options)
		}
		seen[label] = true
	}
	if len(seen) != 3 {
		t.Fatalf("expected three distinct options, got %v", a.deviceSelect.Options)
	}
	if id := a.deviceIDFor(a.deviceSelect.Options[2]); id != "two" {
		t.Fatalf("the second USB Audio option maps to %q", id)
	}
	if id := a.deviceIDFor(a.deviceSelect.Options[3]); id != "three" {
		t.Fatalf("the third USB Audio option maps to %q", id)
	}
}

func TestDeviceIDsMapBackToOptions(t *testing.T) {
	a := newTestApp(t)
	a.applyDevices([]audio.OutputDevice{
		{ID: "abc", Name: "Speakers", MaxRate: 48000, Default: true},
		{ID: "def", Name: "Headphones", MaxRate: 96000},
	})

	if id := a.deviceIDFor(defaultDeviceOption); id != "" {
		t.Fatalf("the system default option maps to %q", id)
	}
	if id := a.deviceIDFor("Headphones · up to 96.0 kHz"); id != "def" {
		t.Fatalf("the headphones option maps to %q", id)
	}
	if id := a.deviceIDFor("nothing like this"); id != "" {
		t.Fatalf("an unknown label mapped to %q", id)
	}
}

func TestADisconnectedDeviceIsStillShown(t *testing.T) {
	devices := []audio.OutputDevice{{ID: "abc", Name: "Speakers", MaxRate: 48000, Default: true}}

	labels, ids := deviceOptions(devices, audio.OutputDevice{})
	if len(labels) != 2 || len(ids) != 2 {
		t.Fatalf("with the system default in use the picker should offer %v, got %v", labels, ids)
	}

	labels, ids = deviceOptions(devices, audio.OutputDevice{ID: "gone", Name: "Old Interface", MaxRate: 44100})
	if len(labels) != 3 {
		t.Fatalf("the vanished device disappeared from the picker: %v", labels)
	}
	last := labels[len(labels)-1]
	if last != "Old Interface · up to 44.1 kHz (disconnected)" {
		t.Fatalf("the vanished device is labelled %q", last)
	}
	if ids[len(ids)-1] != "gone" {
		t.Fatalf("the vanished device maps to %q", ids[len(ids)-1])
	}
}

func TestThePickerIsPopulatedFromTheMachine(t *testing.T) {
	fyneApp := test.NewApp()
	t.Cleanup(fyneApp.Quit)

	devices, err := audio.OutputDevices()
	if err != nil || len(devices) == 0 {
		t.Skipf("no output devices available: %v", err)
	}

	a := New(fyneApp, nil, "")
	if len(a.deviceSelect.Options) != len(devices)+1 {
		t.Fatalf("the picker offers %v for %d devices", a.deviceSelect.Options, len(devices))
	}
	if a.deviceSelect.Selected != defaultDeviceOption {
		t.Fatalf("a fresh app is not on the system default: %q", a.deviceSelect.Selected)
	}
}

func TestSwitchingToARealDeviceMovesTheEngineAndRemembersIt(t *testing.T) {
	fyneApp := test.NewApp()
	t.Cleanup(fyneApp.Quit)

	devices, err := audio.OutputDevices()
	if err != nil || len(devices) == 0 {
		t.Skipf("no output devices available: %v", err)
	}

	a := New(fyneApp, nil, "")
	target := devices[0]
	a.switchDevice(target.ID)
	t.Cleanup(func() {
		if a.engine != nil {
			a.engine.Close()
		}
	})

	if a.engine == nil {
		t.Fatal("switching to a real device left the app without an engine")
	}
	if got := a.engine.Device().ID; got != target.ID {
		t.Fatalf("the engine is on device %q, expected %q", got, target.ID)
	}
	if a.engine.Volume() <= 0 {
		t.Fatalf("the replacement engine came up silent (volume %v)", a.engine.Volume())
	}
	if got := RememberedDevice(fyneApp); got != target.ID {
		t.Fatalf("the choice was remembered as %q, expected %q", got, target.ID)
	}

	before := a.engine
	a.switchDevice(target.ID)
	if a.engine != before {
		t.Fatal("picking the device already in use should not reopen it")
	}
}
