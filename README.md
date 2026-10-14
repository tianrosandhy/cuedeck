# CueDeck

A native desktop cue player for music directors. Load a track, tell it the tempo, drop
cues on beats, and during the show fire a cue to jump straight into that part of the
music — with the jump applied inside the very next audio block, not after a fade or a
decode.

Written in Go, native window (Fyne) + native audio device via miniaudio/CoreAudio, no
browser or Electron layer.

## Build and run

```sh
go build -o cuedeck .
./cuedeck                      # empty session, then use Open
./cuedeck rehearsal.mp3        # open a track straight away
./cuedeck set1.cuedeck         # open a saved cue sheet
```

Flags:

| flag | default | meaning |
| --- | --- | --- |
| `-period` | `128` | audio block size in frames. 128 frames @48 kHz = 2.67 ms. Lower means tighter cue timing, higher is more resilient on a loaded machine. |
| `-selftest <file>` | | play a file for a few seconds, measure decode, callback timing and cue latency, then exit |
| `-window` | `3` | seconds of playback used by `-selftest` for the stability check |

Requires Go 1.26+, a C toolchain (Fyne needs it for OpenGL), and macOS/Linux/Windows.

## The app icon on macOS

macOS takes the Dock and Cmd+Tab icon from the **application bundle**, not from the running
process. That is why `./cuedeck` shows a generic icon no matter what the app does at
runtime: `fyne.App.SetIcon` cannot change it on macOS (the window-icon call behind it is a
no-op there — the app still makes it, because that is what paints the icon on Linux and
Windows).

To get a real icon, run the app from a bundle:

```sh
./packaging/macos/build-app.sh     # writes dist/CueDeck.app
open dist/CueDeck.app
```

The script builds the binary into `dist/CueDeck.app/Contents/MacOS/CueDeck`, draws the icon
with `go run ./tools/icon`, converts it to `Contents/Resources/CueDeck.icns` with the
system `iconutil`, installs `packaging/macos/Info.plist`, and ad-hoc signs the bundle. The
bundle id is `com.cuedeck.desktop`, the same id the app uses for its saved preferences, so
your remembered output device and window settings carry over between the two.

Move `dist/CueDeck.app` into `/Applications` (or anywhere permanent) and the Dock, Cmd+Tab
and Spotlight all keep the icon. Launching the binary inside the bundle directly works too,
since macOS resolves the enclosing bundle either way.

The artwork is generated in Go by `tools/icon/main.go`, so there are no binary assets to
download: a dark rounded square in the Big Sur proportions, the blue and amber cue bars with
the white playhead from the cue map. The same art at 256 px is embedded in the binary
(`internal/ui/icon.png`, via `go:embed`) as the runtime window icon. Re-render it any time
with `go run ./tools/icon -iconset dist/CueDeck.iconset -png internal/ui/icon.png -size 256`.

## Output devices

The `Output` picker in the top right lists every playback device the system reports, with
the highest rate each one offers. Pick one and the engine moves to it; `System default`
follows whatever the OS is using, so it survives you unplugging the interface you had
selected. The small ⟳ button re-scans the device list — plug a new interface in, hit it, and
it appears. The one you pick is remembered, so the next launch opens on the same device (and
falls back to the system default with a note on stderr if it is not there).

The engine asks the device for its own sample rate rather than imposing one, so the audio
callback runs at exactly the rate the hardware is running at, with no conversion inserted by
the backend. Switching devices carries the transport state with it — position, play/pause
and volume — and if the new device runs at a different rate the decoded audio is rebuilt for
that rate (a few seconds on a long track, behind a progress dialog); when the rate matches,
the swap is instant and the audio is not touched at all.

## The workflow

1. **Open** an audio file (`mp3`, `wav`, `flac`, `ogg`) — it is decoded into RAM once, so
   afterwards any position is instantly addressable.
2. **Set the tempo**: type the BPM, or hit **Tap** in time with the track. Set the
   signature if it is not 4/4, and press **Beat 1 = playhead** while the track sits on the
   first downbeat so the bar/beat grid lines up with the music.
3. **Add cues**: `Add cue at playhead` (or the `m` key) drops a cue on the current beat,
   `Add cue at beat…` lets you type an exact beat position. Cues store a *beat*, not a time,
   so they stay locked to the music if you change the BPM later. `snap` rounds new cues to
   the nearest ¼ beat; the edit dialog has a `Snap to nearest beat` button and a
   `Play from here` audition button.
4. **Fire cues** during the show:

   - `1` … `9` fire the first nine cues instantly, or click a row's `Fire` button.
   - `↑` / `↓` *pick* a cue without playing anything — the picked row turns amber and its
     section is highlighted on the strip — then `enter` plays from it. This works for any
     number of cues, so a song with 30 markers is no problem, and it lets you line up the
     next part while the current one is still playing.
   - The part playing right now always carries a blue edge, independent of what is picked.

5. **Save** as a `.cuedeck` file — the audio and all cue data live in one file you can
   carry to the show machine and open again.

The strip under the transport is the cue map: it shows the whole track, each cue as a tick,
each section tinted alternately, the played part in blue and the playhead as a white line.
Click or drag anywhere on it to scrub, scroll over it to nudge by half a beat.

## Keys

| key | action |
| --- | --- |
| `space` | play / pause |
| `esc` | stop (return to beat 1) |
| `1`…`9` | fire cue 1-9 (instant jump) |
| `↑` / `↓` | pick the next / previous cue to play |
| `enter` | play from the picked cue (picks the current part if nothing is picked) |
| `←` / `→` | nudge one beat |
| `,` / `.` | nudge ¼ beat |
| `[` / `]` | nudge one bar |
| `m` | add a cue at the playhead |
| `s` | stop |

Clicking a cue row picks it, and hovering a row lightens it, so the list also works with
the mouse alone.

While the BPM field has the keyboard, the window's keys belong to that field — Fyne hands
keystrokes to whatever holds focus. Pressing `enter` releases the field, and so does
clicking anywhere else, so the transport gets its keys back either way; the status bar
says so while the field is holding them.

## Why triggers have no delay

The requirement is that firing a cue must not be late — a director cannot wait for a
decoder. Four things make that work:

1. **Everything is decoded up front.** A file is decoded and resampled to the device rate
   once, into interleaved float32 PCM in RAM (about 23 MB per stereo minute at 48 kHz).
   Firing a cue never touches the disk, a decoder, or an allocator.
2. **The audio callback owns the play position.** The render callback reads from the PCM
   buffer at an atomic frame index and advances it. Nothing else ever blocks or allocates
   inside the callback.
3. **A trigger is one atomic store.** `SeekSeconds` publishes a pending frame index with a
   single atomic store and returns; the next render callback swaps it in and continues from
   there. The trigger path never waits for a lock, a buffer, or a device.
4. **Silence, not stop, when paused.** The device runs for the whole session, so resuming
   and jumping never pay device start-up latency.

Measured on a MacBook Pro (48 kHz, `-period 128`):

```
block requested : 128 frames (2.67 ms)
block measured  : 2.67 ms (128 frames)
cue 1           : asked 8.000 s, heard 0.248 s -> applied 35µs later, position now 8.083 s
cue 2           : asked 12.000 s, heard 8.083 s -> applied 1.677ms later, position now 12.077 s
cue 3           : asked 16.000 s, heard 12.077 s -> applied 310µs later, position now 16.080 s
cue 4           : asked 20.000 s, heard 16.080 s -> applied 709µs later, position now 20.080 s
cue 5           : asked 24.000 s, heard 20.080 s -> applied 1.103ms later, position now 24.080 s
cue 6           : asked 28.000 s, heard 24.080 s -> applied 758µs later, position now 28.080 s
callback rate   : 1876 callbacks in 5.001s (expected 1890)
callback spread : min 2.56ms / max 2.779ms across 1876 blocks
```

So a triggered jump reaches the audio path in well under one block (35 µs to 1.7 ms, worst
case one block), and the callback spread shows the engine ran without a single late or
missing block. What remains is the physical floor of digital audio: the block already in
flight plus the device's own buffer, which is why the default is 128 frames
(2.67 ms per block). Reproduce with:

```sh
./cuedeck -selftest rehearsal.mp3 -period 128
```

`-period 64` halves the block to 1.33 ms if you want the tightest possible cue; the status
bar always shows the *measured* block size and the latency of the last cue, so you can see
what your machine actually does rather than trusting the configured number.

## Stereo

Stereo is kept end to end: the decoder yields left and right separately, the engine stores
interleaved stereo PCM and writes both channels in the render callback. A mono source is
duplicated into both channels, which is what every other player does with it.

Two tests pin this down rather than assuming it. One builds a stereo WAV with a 440 Hz tone
on the left and 220 Hz on the right, decodes it through the same path as a real file
(including resampling 44.1 kHz → 48 kHz) and checks with a Goertzel filter that each channel
still holds *its own* tone, so a channel swap or a collapse to mono would fail. The other
decodes the bundled `mp3example.mp3` (2 ch, 44.1 kHz) and checks both channels are present
and different — measured left/right RMS 0.135/0.137 with a mean channel difference of 0.078.

## Project files

A `.cuedeck` file is a container: `CUEDECK\x01` magic, a length-prefixed JSON header, then
the original encoded audio. The header holds the tempo (BPM, beat-1 offset, signature), the
cue list (id, label, beat), and the audio's original name/format/sample rate/size. Saving
writes to a temporary file and renames it, so a crash mid-save cannot destroy the previous
version, and loading verifies the header before trusting any of it.

## Layout

```
main.go                    flags, self-test, wiring
internal/audio/decode.go   mp3/wav/flac/ogg -> float32 PCM at the device rate
internal/audio/engine.go   miniaudio device, render callback, instant jumps, metering
internal/model/project.go  tempo and beat maths, cue list
internal/project/file.go   .cuedeck reader/writer
internal/ui/app.go         window, transport, tempo bar, keyboard, file handling
internal/ui/devices.go     output device list, switching, remembering the choice
internal/ui/playhead.go    the cue map strip widget
internal/ui/markers.go     cue rows and cue dialogs
tools/icon/main.go         draws the app icon (all iconset sizes plus the embedded PNG)
packaging/macos/           Info.plist and the script that assembles CueDeck.app
```

## Tests

```sh
go test ./...
```

Cue maths, the project file round trip, decoding of `mp3example.mp3`, stereo channel
separation, the render callback (jump latency, seek clamping, end of track, callback
stability), output device listing and switching, canvas gestures, cue hotkeys, arrow
picking, hover and click states, focus handling and pixel checks on the cue map and on the
row highlights. The device-dependent audio tests skip themselves when no output device is
available.

## Known limits

- Decoding a long track takes a moment (a 2.5 minute MP3 takes about 3 s: ~1.8 s to decode
  and ~1.4 s to resample 44.1 kHz → 48 kHz). A progress dialog covers it. Audio already at
  the device rate skips the resample. Switching to an output device that runs at a different
  rate pays that cost again.
- Cues jump immediately; there is no quantise-to-the-next-beat option.
- Cue files always embed the audio, so a `.cuedeck` for a long WAV is large — that is the
  price of one self-contained file.
- If an output device disappears mid-show the engine keeps running against it (and the
  picker marks it `disconnected`); pick another device to recover.
