package audio

import (
	"strings"
	"testing"
	"time"
)

func TestOutputDevicesAreListed(t *testing.T) {
	devices, err := OutputDevices()
	if err != nil {
		t.Skipf("no audio backend available: %v", err)
	}
	if len(devices) == 0 {
		t.Skip("no output devices found")
	}

	seen := map[string]bool{}
	for _, device := range devices {
		if device.Name == "" {
			t.Fatalf("a device was listed without a name: %+v", device)
		}
		if device.ID == "" {
			t.Fatalf("%s was listed without an id", device.Name)
		}
		if seen[device.ID] {
			t.Fatalf("device id %s was listed twice", device.ID)
		}
		seen[device.ID] = true
		if device.MaxRate < 0 {
			t.Fatalf("%s was listed with a negative sample rate", device.Name)
		}
		t.Logf("output device: %s · up to %d Hz (default %v)", device.Name, device.MaxRate, device.Default)
	}
}

func TestStartOnTheSystemDefault(t *testing.T) {
	engine, err := StartOn("", 128)
	if err != nil {
		t.Skipf("cannot start the audio engine: %v", err)
	}
	defer engine.Close()

	device := engine.Device()
	if device.ID != "" {
		t.Fatalf("the system default engine reports device id %q", device.ID)
	}
	if device.Name == "" {
		t.Fatal("the engine did not resolve a device name")
	}
	if engine.Rate() <= 0 {
		t.Fatalf("the engine reports a sample rate of %d", engine.Rate())
	}
	if engine.ConfiguredPeriod() != 128 {
		t.Fatalf("the engine reports a period of %d", engine.ConfiguredPeriod())
	}

	engine.ResetStats()
	time.Sleep(200 * time.Millisecond)
	if engine.Blocks() == 0 {
		t.Fatal("the engine never ran its callback")
	}
	if frames := engine.BlockFrames(); frames < 120 || frames > 136 {
		t.Fatalf("the device is using %d frame blocks, expected roughly the requested 128", frames)
	}
}

func TestStartOnASpecificDevice(t *testing.T) {
	devices, err := OutputDevices()
	if err != nil || len(devices) == 0 {
		t.Skipf("no output devices available: %v", err)
	}
	target := devices[0]

	engine, err := StartOn(target.ID, 256)
	if err != nil {
		t.Fatalf("starting on %q failed: %v", target.Name, err)
	}
	defer engine.Close()

	device := engine.Device()
	if device.ID != target.ID {
		t.Fatalf("the engine reports device %q, expected %q", device.ID, target.ID)
	}
	if device.Name != target.Name {
		t.Fatalf("the engine reports device %q, expected %q", device.Name, target.Name)
	}
	if engine.Rate() <= 0 {
		t.Fatalf("the engine reports a sample rate of %d", engine.Rate())
	}
	if target.MaxRate > 0 && engine.Rate() > target.MaxRate {
		t.Fatalf("the engine runs at %d Hz, above the %d Hz the device offers", engine.Rate(), target.MaxRate)
	}
	if engine.ConfiguredPeriod() != 256 {
		t.Fatalf("the engine reports a period of %d", engine.ConfiguredPeriod())
	}
}

func TestStartOnAMissingDeviceFails(t *testing.T) {
	engine, err := StartOn("a-device-that-is-not-here", 128)
	if err == nil {
		engine.Close()
		t.Fatal("starting on a missing device should fail")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAReplacementEnginePicksUpTheTransportState(t *testing.T) {
	first, err := StartOn("", 128)
	if err != nil {
		t.Skipf("cannot start the audio engine: %v", err)
	}
	t.Cleanup(first.Close)

	clip := &Clip{
		Data:   make([]float32, first.Rate()*4*2),
		Rate:   first.Rate(),
		Frames: int64(first.Rate() * 4),
		Kind:   "TEST",
	}
	first.Load(clip)
	first.Play()
	time.Sleep(120 * time.Millisecond)
	first.Pause()

	second, err := StartOn("", 128)
	if err != nil {
		t.Fatalf("a second engine could not be opened: %v", err)
	}
	defer second.Close()

	second.SetVolume(0.4)
	second.Load(clip)
	second.SeekSeconds(2)
	second.Play()
	time.Sleep(120 * time.Millisecond)

	if !second.IsPlaying() {
		t.Fatal("the replacement engine did not start")
	}
	if position := second.Position(); position < 1.9 || position > 2.3 {
		t.Fatalf("the replacement engine sits at %v, expected the position it was handed", position)
	}
	if second.MeasuredBlock() <= 0 {
		t.Fatal("the replacement engine is not running its callback")
	}
	first.Close()
}
