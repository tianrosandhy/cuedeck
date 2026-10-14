package main

import (
	"bytes"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2/app"

	"cuedeck/internal/audio"
	"cuedeck/internal/project"
	"cuedeck/internal/ui"
)

func main() {
	period := flag.Int("period", 0, "output block size in frames, 0 uses the last one you picked (default 128)")
	memory := flag.Int("memory", 2048, "megabytes of decoded audio to keep for the loaded projects")
	selftest := flag.String("selftest", "", "measure decode, playback and cue latency for an audio or .cuedeck file, then exit")
	window := flag.Int("window", 3, "seconds of playback used by -selftest to check the audio callback")
	device := flag.String("device", "", "output device for -selftest, matched against the device name")
	flag.Usage = usage
	flag.Parse()

	if *selftest != "" {
		if err := selfTest(*selftest, chosenBlock(*period), *window, *device); err != nil {
			fmt.Fprintf(os.Stderr, "selftest failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fyneApp := app.NewWithID("com.cuedeck.desktop")
	deviceID := ui.RememberedDevice(fyneApp)
	engine, err := audio.StartOn(deviceID, ui.RememberedBlock(fyneApp, chosenBlock(*period)))
	if err != nil && deviceID != "" {
		fmt.Fprintf(os.Stderr, "output device %q is unavailable (%v), using the system default\n", deviceID, err)
		engine, err = audio.StartOn("", ui.RememberedBlock(fyneApp, chosenBlock(*period)))
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

func chosenBlock(requested int) int {
	if requested > 0 {
		return requested
	}
	return audio.DefaultPeriodFrames
}

func selfTest(path string, period, window int, wanted string) error {
	devices, err := audio.OutputDevices()
	if err != nil {
		return err
	}
	fmt.Printf("output devices  : %d found\n", len(devices))
	for _, listed := range devices {
		marker := ""
		if listed.Default {
			marker = " (default)"
		}
		upTo := ""
		if listed.MaxRate > 0 {
			upTo = fmt.Sprintf(" · up to %.1f kHz", float64(listed.MaxRate)/1000)
		}
		fmt.Printf("                  %s%s%s\n", listed.Name, upTo, marker)
	}

	deviceID := ""
	if wanted != "" {
		for _, listed := range devices {
			if strings.Contains(strings.ToLower(listed.Name), strings.ToLower(wanted)) {
				deviceID = listed.ID
				break
			}
		}
		if deviceID == "" {
			return fmt.Errorf("no output device matches %q", wanted)
		}
	}

	engine, err := audio.StartOn(deviceID, period)
	if err != nil {
		return err
	}
	defer engine.Close()

	device := engine.Device()
	fmt.Printf("device          : %s\n", device.Name)
	fmt.Printf("engine rate     : %d Hz (negotiated with the device)\n", engine.Rate())
	fmt.Printf("block requested : %d frames (%.2f ms)\n", period, 1000*float64(period)/float64(engine.Rate()))

	name := filepath.Base(path)
	var raw []byte
	if project.IsProjectFile(path) {
		loaded, payload, err := project.Load(path)
		if err != nil {
			return err
		}
		raw = payload
		name = loaded.Audio.OriginalName
		fmt.Printf("project         : %s (%d cues, %.0f bpm)\n", loaded.Name, len(loaded.Markers), loaded.Tempo.BPM)
	} else {
		raw, err = os.ReadFile(path)
		if err != nil {
			return err
		}
	}
	if name == "" {
		name = filepath.Base(path)
	}

	start := time.Now()
	clip, err := audio.Decode(name, bytes.NewReader(raw), engine.Rate())
	if err != nil {
		return err
	}
	fmt.Printf("decoded         : %s %s, %d Hz -> %d Hz, %d frames, %.2f s, PCM %.1f MB in %s\n",
		clip.Kind, name, clip.SourceRate, clip.Rate, clip.Frames, clip.Seconds(),
		float64(len(clip.Data))*4/float64(1<<20), time.Since(start).Round(time.Millisecond))

	peak, sum := 0.0, 0.0
	for _, sample := range clip.Data {
		value := float64(sample)
		if magnitude := math.Abs(value); magnitude > peak {
			peak = magnitude
		}
		sum += value * value
	}
	fmt.Printf("file level      : peak %.4f (%.2f dBFS), rms %.4f (%.2f dBFS)\n",
		peak, 20*math.Log10(peak+1e-12), math.Sqrt(sum/float64(len(clip.Data))), 20*math.Log10(math.Sqrt(sum/float64(len(clip.Data)))+1e-12))

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
	positionBefore := engine.Position()
	windowStart := time.Now()
	time.Sleep(time.Duration(window) * time.Second)
	elapsed := time.Since(windowStart)
	positionAfter := engine.Position()
	blocks := engine.Blocks()
	expected := elapsed.Seconds() * float64(engine.Rate()) / blockFrames
	low, high := engine.BlockSpread()
	outPeak := engine.TakePeak()
	fmt.Printf("callback rate   : %d callbacks in %s (expected %.0f)\n", blocks, elapsed.Round(time.Millisecond), expected)
	fmt.Printf("callback spread : min %s / max %s across %d blocks\n", low.Round(time.Microsecond), high.Round(time.Microsecond), blocks)
	played := (positionAfter - positionBefore) / elapsed.Seconds()
	fmt.Printf("clock           : %.3f s of audio advanced in %.3f s of wall clock (%+.3f%% off real time)\n",
		positionAfter-positionBefore, elapsed.Seconds(), (played-1)*100)
	fmt.Printf("late blocks     : %d (worst %s behind schedule)\n", engine.LateBlocks(), engine.WorstLateBlock().Round(time.Microsecond))
	fmt.Printf("output peak     : %.4f (%.2f dBFS), samples at full scale %d\n",
		outPeak, 20*math.Log10(float64(outPeak)+1e-12), engine.FullScaleSamples())
	if float64(blocks) < expected*0.95 {
		fmt.Printf("warning         : the device delivered fewer blocks than expected, try a larger -period\n")
	}
	if engine.LateBlocks() > 0 {
		fmt.Printf("warning         : the audio callback fell behind %d times, which is what crackling sounds like; try a larger -period\n", engine.LateBlocks())
	}
	if drift := played - 1; drift > 0.005 || drift < -0.005 {
		fmt.Printf("warning         : the audio did not play at real time, something between here and the speakers is resampling\n")
	}
	return nil
}
