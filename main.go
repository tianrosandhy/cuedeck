package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2/app"

	"cuedeck/internal/audio"
	"cuedeck/internal/ui"
)

func main() {
	period := flag.Int("period", audio.DefaultPeriodFrames, "output block size in frames, lower means tighter cue timing")
	memory := flag.Int("memory", 2048, "megabytes of decoded audio to keep for the loaded projects")
	selftest := flag.String("selftest", "", "measure decode, playback and cue latency for an audio file, then exit")
	window := flag.Int("window", 3, "seconds of playback used by -selftest to check the audio callback")
	flag.Usage = usage
	flag.Parse()

	if *selftest != "" {
		if err := selfTest(*selftest, *period, *window); err != nil {
			fmt.Fprintf(os.Stderr, "selftest failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fyneApp := app.NewWithID("com.cuedeck.desktop")
	deviceID := ui.RememberedDevice(fyneApp)
	engine, err := audio.StartOn(deviceID, *period)
	if err != nil && deviceID != "" {
		fmt.Fprintf(os.Stderr, "output device %q is unavailable (%v), using the system default\n", deviceID, err)
		engine, err = audio.StartOn("", *period)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot start the audio engine: %v\n", err)
		os.Exit(1)
	}
	defer engine.Close()

	application := ui.New(fyneApp, engine, ui.Options{
		Paths:        flag.Args(),
		MemoryBudget: int64(*memory) << 20,
	})
	application.Run()
}

func usage() {
	fmt.Fprintf(os.Stderr, "CueDeck - cue player for music directors\n\n")
	fmt.Fprintf(os.Stderr, "usage:\n  cuedeck [flags] [project or audio file ...]\n\nflags:\n")
	flag.PrintDefaults()
}

func selfTest(path string, period, window int) error {
	devices, err := audio.OutputDevices()
	if err != nil {
		return err
	}
	fmt.Printf("output devices  : %d found\n", len(devices))
	for _, device := range devices {
		marker := ""
		if device.Default {
			marker = " (default)"
		}
		upTo := ""
		if device.MaxRate > 0 {
			upTo = fmt.Sprintf(" · up to %.1f kHz", float64(device.MaxRate)/1000)
		}
		fmt.Printf("                  %s%s%s\n", device.Name, upTo, marker)
	}

	engine, err := audio.StartOn("", period)
	if err != nil {
		return err
	}
	defer engine.Close()

	device := engine.Device()
	fmt.Printf("device          : %s\n", device.Name)
	fmt.Printf("engine rate     : %d Hz (negotiated with the device)\n", engine.Rate())
	fmt.Printf("block requested : %d frames (%.2f ms)\n", period, 1000*float64(period)/float64(engine.Rate()))

	start := time.Now()
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	clip, err := audio.Decode(filepath.Base(path), bytes.NewReader(raw), engine.Rate())
	if err != nil {
		return err
	}
	fmt.Printf("decoded         : %s %s, %d Hz -> %d Hz, %d frames, %.2f s, PCM %.1f MB in %s\n",
		clip.Kind, filepath.Base(path), clip.SourceRate, clip.Rate, clip.Frames, clip.Seconds(),
		float64(len(clip.Data))*4/float64(1<<20), time.Since(start).Round(time.Millisecond))

	engine.Load(clip)
	time.Sleep(500 * time.Millisecond)
	if block := engine.MeasuredBlock(); block > 0 {
		frames := float64(block) * float64(engine.Rate()) / float64(time.Second)
		fmt.Printf("block measured  : %.2f ms (%.0f frames)\n", float64(block.Microseconds())/1000, frames)
	}

	engine.Play()
	time.Sleep(250 * time.Millisecond)
	fmt.Printf("playing         : %v at %.3f s\n", engine.IsPlaying(), engine.Position())

	var worst time.Duration
	for i := 0; i < 6; i++ {
		target := 8 + float64(i)*4
		if target >= clip.Seconds()*0.95 {
			target = clip.Seconds() * 0.9
		}
		before := engine.Position()
		engine.SeekSeconds(target)
		time.Sleep(80 * time.Millisecond)
		latency := engine.CueLatency()
		if latency > worst {
			worst = latency
		}
		fmt.Printf("cue %d           : asked %.3f s, heard %.3f s -> applied %s later, position now %.3f s\n",
			i+1, target, before, latency.Round(time.Microsecond), engine.Position())
	}
	fmt.Printf("worst cue apply : %s (the jump is applied inside the next audio block)\n", worst.Round(time.Microsecond))

	fmt.Printf("playing         : %v at %.3f s of %.3f s\n", engine.IsPlaying(), engine.Position(), clip.Seconds())

	blockFrames := float64(engine.BlockFrames())
	engine.ResetStats()
	windowStart := time.Now()
	time.Sleep(time.Duration(window) * time.Second)
	elapsed := time.Since(windowStart)
	blocks := engine.Blocks()
	expected := elapsed.Seconds() * float64(engine.Rate()) / blockFrames
	low, high := engine.BlockSpread()
	fmt.Printf("callback rate   : %d callbacks in %s (expected %.0f)\n", blocks, elapsed.Round(time.Millisecond), expected)
	fmt.Printf("callback spread : min %s / max %s across %d blocks\n", low.Round(time.Microsecond), high.Round(time.Microsecond), blocks)
	if float64(blocks) < expected*0.95 {
		fmt.Printf("warning         : the device delivered fewer blocks than expected, try a larger -period\n")
	}
	return nil
}
