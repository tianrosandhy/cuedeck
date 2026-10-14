package audio

import (
	"fmt"
	"math"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/gen2brain/malgo"
)

const DefaultPeriodFrames = 128

type OutputDevice struct {
	ID      string
	Name    string
	MaxRate int
	Default bool
}

func OutputDevices() ([]OutputDevice, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()
	return listDevices(ctx)
}

func listDevices(ctx *malgo.AllocatedContext) ([]OutputDevice, error) {
	devices, err := ctx.Devices(malgo.Playback)
	if err != nil {
		return nil, err
	}
	listed := make([]OutputDevice, 0, len(devices))
	for i := range devices {
		listed = append(listed, describeDevice(ctx, &devices[i]))
	}
	return listed, nil
}

func describeDevice(ctx *malgo.AllocatedContext, info *malgo.DeviceInfo) OutputDevice {
	device := OutputDevice{
		ID:      info.ID.String(),
		Name:    info.Name(),
		Default: info.IsDefault != 0,
	}
	if detailed, err := ctx.DeviceInfo(malgo.Playback, info.ID, malgo.Shared); err == nil {
		for _, format := range detailed.Formats {
			if rate := int(format.SampleRate); rate > device.MaxRate {
				device.MaxRate = rate
			}
		}
	}
	return device
}

type Engine struct {
	ctx    *malgo.AllocatedContext
	dev    *malgo.Device
	device OutputDevice
	rate   int
	period int

	clip     atomic.Pointer[Clip]
	pos      atomic.Int64
	pending  atomic.Int64
	askedAt  atomic.Int64
	playing  atomic.Bool
	ended    atomic.Bool
	volume   atomic.Uint32
	cueNs    atomic.Int64
	blockNs  atomic.Int64
	blockMin atomic.Int64
	blockMax atomic.Int64
	blocks   atomic.Int64
	late     atomic.Int64
	lateNs   atomic.Int64
	peak     atomic.Uint32
	full     atomic.Int64
	lastCb   atomic.Int64

	gain float32
}

func StartOn(id string, periodFrames int) (*Engine, error) {
	if periodFrames <= 0 {
		periodFrames = DefaultPeriodFrames
	}
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, err
	}

	devices, err := ctx.Devices(malgo.Playback)
	if err != nil {
		devices = nil
	}

	device := OutputDevice{Name: "system default", Default: true}
	var chosen *malgo.DeviceID
	if id == "" {
		for i := range devices {
			if candidate := describeDevice(ctx, &devices[i]); candidate.Default {
				device = candidate
				break
			}
		}
	} else {
		for i := range devices {
			if candidate := describeDevice(ctx, &devices[i]); candidate.ID == id {
				device = candidate
				chosen = &devices[i].ID
				break
			}
		}
		if chosen == nil {
			_ = ctx.Uninit()
			ctx.Free()
			return nil, fmt.Errorf("output device %q is not connected", id)
		}
	}
	device.ID = id

	engine := &Engine{ctx: ctx, device: device, period: periodFrames}
	engine.pending.Store(-1)
	engine.volume.Store(math.Float32bits(1))

	config := malgo.DefaultDeviceConfig(malgo.Playback)
	config.PeriodSizeInFrames = uint32(periodFrames)
	config.PerformanceProfile = malgo.LowLatency
	config.Playback.Format = malgo.FormatF32
	config.Playback.Channels = 2
	config.Playback.ShareMode = malgo.Shared
	if chosen != nil {
		config.Playback.DeviceID = chosen.Pointer()
	}

	handle, err := malgo.InitDevice(ctx.Context, config, malgo.DeviceCallbacks{Data: engine.onData})
	if err != nil {
		_ = ctx.Uninit()
		ctx.Free()
		return nil, err
	}
	engine.rate = int(handle.PlaybackInternalSampleRate())
	if engine.rate <= 0 {
		engine.rate = int(handle.SampleRate())
	}
	if engine.rate <= 0 {
		engine.rate = 48000
	}
	if err := handle.Start(); err != nil {
		handle.Uninit()
		_ = ctx.Uninit()
		ctx.Free()
		return nil, err
	}
	engine.dev = handle
	return engine, nil
}

func (e *Engine) onData(output, _ []byte, frames uint32) {
	now := time.Now().UnixNano()
	if last := e.lastCb.Swap(now); last > 0 {
		delta := now - last
		if prev := e.blockNs.Load(); prev == 0 {
			e.blockNs.Store(delta)
			e.blockMin.Store(delta)
			e.blockMax.Store(delta)
		} else {
			e.blockNs.Store(prev + (delta-prev)/8)
			if previous := e.blockMin.Load(); previous == 0 || delta < previous {
				e.blockMin.Store(delta)
			}
			if previous := e.blockMax.Load(); delta > previous {
				e.blockMax.Store(delta)
			}
			if delta > prev+prev/2 {
				e.late.Add(1)
				if late := delta - prev; late > e.lateNs.Load() {
					e.lateNs.Store(late)
				}
			}
		}
	}
	e.blocks.Add(1)

	if target := e.pending.Swap(-1); target >= 0 {
		if asked := e.askedAt.Swap(0); asked > 0 {
			e.cueNs.Store(now - asked)
		}
		e.pos.Store(target)
		e.ended.Store(false)
	}

	if len(output) < 8 {
		return
	}
	samples := unsafe.Slice((*float32)(unsafe.Pointer(&output[0])), len(output)/4)
	gain := e.gainTarget()

	if !e.playing.Load() {
		clear(samples)
		e.gain = gain
		return
	}

	clip := e.clip.Load()
	if clip == nil {
		clear(samples)
		e.playing.Store(false)
		e.gain = gain
		return
	}

	pos := e.pos.Load()
	count := int64(frames)
	if remain := clip.Frames - pos; count > remain {
		count = remain
	}
	if count <= 0 {
		clear(samples)
		e.playing.Store(false)
		e.ended.Store(true)
		e.gain = gain
		return
	}

	previous := e.gain
	data := clip.Data
	var peak float32
	var full int64
	for i := int64(0); i < count; i++ {
		current := gain
		if previous != gain {
			current = previous + (gain-previous)*float32(i)/float32(count)
		}
		source := (pos + i) * 2
		left := data[source] * current
		right := data[source+1] * current
		samples[2*i] = left
		samples[2*i+1] = right
		if magnitude := magnitude32(left); magnitude > peak {
			peak = magnitude
		}
		if magnitude := magnitude32(right); magnitude > peak {
			peak = magnitude
		}
		if magnitude32(left) >= 1 {
			full++
		}
		if magnitude32(right) >= 1 {
			full++
		}
	}
	if tail := int(count) * 2; tail < len(samples) {
		clear(samples[tail:])
	}
	e.gain = gain
	e.pos.Store(pos + count)
	e.recordLevel(peak, full)

	if count < int64(frames) {
		e.playing.Store(false)
		e.ended.Store(pos+count >= clip.Frames)
	}
}

func (e *Engine) gainTarget() float32 {
	return math.Float32frombits(e.volume.Load())
}

func magnitude32(value float32) float32 {
	if value < 0 {
		return -value
	}
	return value
}

func (e *Engine) recordLevel(peak float32, full int64) {
	if full > 0 {
		e.full.Add(full)
	}
	if peak <= 0 {
		return
	}
	for {
		current := e.peak.Load()
		if peak <= math.Float32frombits(current) {
			return
		}
		if e.peak.CompareAndSwap(current, math.Float32bits(peak)) {
			return
		}
	}
}

func (e *Engine) TakePeak() float32 {
	if e == nil {
		return 0
	}
	return math.Float32frombits(e.peak.Swap(0))
}

func (e *Engine) FullScaleSamples() int64 {
	if e == nil {
		return 0
	}
	return e.full.Load()
}

func (e *Engine) LateBlocks() int64 {
	if e == nil {
		return 0
	}
	return e.late.Load()
}

func (e *Engine) WorstLateBlock() time.Duration {
	if e == nil {
		return 0
	}
	return time.Duration(e.lateNs.Load())
}

func (e *Engine) Device() OutputDevice {
	if e == nil {
		return OutputDevice{}
	}
	return e.device
}

func (e *Engine) Rate() int {
	if e == nil {
		return 0
	}
	return e.rate
}

func (e *Engine) ConfiguredPeriod() int {
	if e == nil {
		return DefaultPeriodFrames
	}
	return e.period
}

func (e *Engine) Load(clip *Clip) {
	if e == nil {
		return
	}
	e.playing.Store(false)
	e.ended.Store(false)
	e.clip.Store(clip)
	e.askedAt.Store(0)
	e.pending.Store(0)
}

func (e *Engine) HasClip() bool {
	if e == nil {
		return false
	}
	return e.clip.Load() != nil
}

func (e *Engine) Clip() *Clip {
	if e == nil {
		return nil
	}
	return e.clip.Load()
}

func (e *Engine) Duration() float64 {
	if e == nil {
		return 0
	}
	return e.clip.Load().Seconds()
}

func (e *Engine) BlockFrames() int64 {
	if e == nil {
		return 1
	}
	block := e.blockNs.Load()
	if block <= 0 {
		return int64(e.rate) / 100
	}
	rate := int64(e.rate)
	frames := int64(math.Round(float64(block) * float64(rate) / float64(time.Second)))
	if frames < 1 {
		frames = 1
	}
	return frames
}

func (e *Engine) MeasuredBlock() time.Duration {
	if e == nil {
		return 0
	}
	if block := e.blockNs.Load(); block > 0 {
		return time.Duration(block)
	}
	return 0
}

func (e *Engine) Blocks() int64 {
	if e == nil {
		return 0
	}
	return e.blocks.Load()
}

func (e *Engine) BlockSpread() (time.Duration, time.Duration) {
	if e == nil {
		return 0, 0
	}
	return time.Duration(e.blockMin.Load()), time.Duration(e.blockMax.Load())
}

func (e *Engine) ResetStats() {
	if e == nil {
		return
	}
	e.blocks.Store(0)
	e.blockMin.Store(0)
	e.blockMax.Store(0)
	e.late.Store(0)
	e.lateNs.Store(0)
	e.peak.Store(0)
	e.full.Store(0)
}

func (e *Engine) CueLatency() time.Duration {
	if e == nil {
		return 0
	}
	if cue := e.cueNs.Load(); cue > 0 {
		return time.Duration(cue)
	}
	return 0
}

func (e *Engine) PositionFrame() int64 {
	if e == nil {
		return 0
	}
	pos := e.pos.Load()
	if e.playing.Load() {
		if pos -= e.BlockFrames(); pos < 0 {
			pos = 0
		}
	}
	return pos
}

func (e *Engine) Position() float64 {
	if e == nil || e.rate <= 0 {
		return 0
	}
	return float64(e.PositionFrame()) / float64(e.rate)
}

func (e *Engine) SeekFrame(frame int64) {
	if e == nil {
		return
	}
	clip := e.clip.Load()
	if clip == nil {
		return
	}
	if frame < 0 {
		frame = 0
	}
	if frame > clip.Frames {
		frame = clip.Frames
	}
	e.ended.Store(false)
	e.askedAt.Store(time.Now().UnixNano())
	e.pending.Store(frame)
}

func (e *Engine) SeekSeconds(seconds float64) {
	if e == nil {
		return
	}
	e.SeekFrame(int64(seconds * float64(e.rate)))
}

func (e *Engine) Play() {
	if e == nil {
		return
	}
	clip := e.clip.Load()
	if clip == nil {
		return
	}
	if e.pos.Load() >= clip.Frames {
		e.SeekFrame(0)
	}
	e.ended.Store(false)
	e.playing.Store(true)
}

func (e *Engine) Pause() {
	if e == nil {
		return
	}
	e.playing.Store(false)
}

func (e *Engine) Toggle() bool {
	if e == nil {
		return false
	}
	if e.playing.Load() {
		e.Pause()
		return false
	}
	e.Play()
	return true
}

func (e *Engine) Stop() {
	if e == nil {
		return
	}
	e.playing.Store(false)
	e.SeekFrame(0)
}

func (e *Engine) IsPlaying() bool {
	if e == nil {
		return false
	}
	return e.playing.Load()
}

func (e *Engine) Ended() bool {
	if e == nil {
		return false
	}
	return e.ended.Load()
}

func (e *Engine) SetVolume(v float64) {
	if e == nil {
		return
	}
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	e.volume.Store(math.Float32bits(float32(v)))
}

func (e *Engine) Volume() float64 {
	if e == nil {
		return 0
	}
	return float64(e.gainTarget())
}

func (e *Engine) Close() {
	if e == nil {
		return
	}
	if e.dev != nil {
		e.playing.Store(false)
		e.dev.Uninit()
		e.dev = nil
	}
	if e.ctx != nil {
		_ = e.ctx.Uninit()
		e.ctx.Free()
		e.ctx = nil
	}
}
