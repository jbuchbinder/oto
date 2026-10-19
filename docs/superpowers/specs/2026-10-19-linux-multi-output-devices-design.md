# Multiple output devices on Linux: per-context device selection

**Status:** approved design
**Date:** 2026-10-19
**Scope:** Linux and the other Unix targets that use the PulseAudio/ALSA backends
(FreeBSD, NetBSD, OpenBSD). Other platforms keep their current single-context
behavior.

## Problem

`oto.NewContext` allows exactly one context per process (`context.go`,
`contextCreated`), and the Unix backends ignore which device they talk to:

- PulseAudio always plays to the server's default sink.
- ALSA walks `deviceCandidates()` — `default`, `plug:default`, then every name
  hint from `snd_device_name_hint` — and keeps the first PCM that opens.

A program that needs to drive two outputs (main mix plus a cue/monitor feed, a
multi-room player, a test rig with two DACs) therefore cannot do it in one
process, and cannot even choose a non-default device in a single process.

## Goals

1. `NewContextOptions.DeviceName` selects the output device for that context;
   empty keeps today's behavior (system default).
2. `NewContextOptions.Backend` (`BackendAuto`, `BackendPulseAudio`,
   `BackendALSA`) resolves names that are ambiguous across the two stacks.
3. `oto.OutputDevices()` lists selectable devices with stable IDs.
4. Several contexts can coexist in one process on Unix, each with its own mux,
   device, sample rate, and lifecycle.
5. Nothing changes for programs that do not set the new fields.

## Non-goals

- Device selection on Windows, macOS, Android, iOS, js, or the console SDKs.
  Those drivers hold package-level context pointers for their C callbacks
  (`driver_darwin.go`, `driver_winmm_windows.go`, `driver_console.go`) and stay
  single-context. Setting the new fields there fails fast.
- Hotplug or reacquisition: losing a device mid-playback stays a fatal per-context
  `Err`, as today.
- Process-wide Suspend/Resume across contexts.
- Automatic sample-rate conversion between contexts: each context keeps its own
  rate, as today.

## Public API

```go
type Backend int

const (
    BackendAuto       Backend = iota // PulseAudio first, ALSA fallback
    BackendPulseAudio                // PulseAudio / PipeWire-pulse only
    BackendALSA                      // ALSA only
)

func (b Backend) String() string // "auto", "pulseaudio", "alsa"

type NewContextOptions struct {
    // ... existing fields ...
    DeviceName string  // empty = system default
    Backend    Backend // default BackendAuto
}

type OutputDevice struct {
    ID      string  // value to pass as DeviceName
    Name    string  // human-readable description
    Backend Backend
}

func OutputDevices() ([]OutputDevice, error)

var (
    ErrDeviceSelectionUnsupported = errors.New("oto: device selection is not supported on this platform")
    ErrDeviceNotFound             = errors.New("oto: output device not found")
)
```

- `OutputDevice.ID` is the PulseAudio sink name (for example
  `alsa_output.pci-0000_00_1f.3.analog-stereo`) or the ALSA PCM name
  (`default`, `hw:1,0`, `plug:hw:1,0`).
- Adding struct fields is source-compatible. Loosening the single-context rule is
  a behavior change only in that previously failing code now succeeds.
- Validation stays synchronous in `NewContext`: an out-of-range `Backend`, or a
  non-zero `DeviceName`/`Backend` on a platform without device selection, returns
  an error immediately (wrapping `ErrDeviceSelectionUnsupported`), next to the
  existing sample-rate and channel-count checks.

## Resolution rules

Implemented in `device_unix.go`, which owns name expansion, sink matching, and
the order of attempts (`resolveOutputDevice`). `driver_unix.go` walks the
attempts and stops early on `ErrDeviceNotFound`. The PulseAudio driver matches a
requested name against the sink list of its own client connection and passes the
matched sink to the playback stream, so selecting a sink costs no extra
connection.

| Backend | DeviceName | Attempts, in order |
|---|---|---|
| Auto | empty | PulseAudio default sink, then ALSA candidate scan |
| PulseAudio | empty | PulseAudio default sink |
| ALSA | empty | ALSA candidate scan |
| PulseAudio | name | sink `name` only |
| ALSA | name | PCM `name`, then `plug:name` when not already plug-prefixed |
| Auto | name | sink `name`; if the sink does not exist, stop with `ErrDeviceNotFound` and a hint to use `BackendALSA`; if PulseAudio is unreachable, ALSA `name` |

Name matching for PulseAudio: exact sink ID, then a case-insensitive exact match
on sink ID, then a case-insensitive exact match on the sink description. A
description that matches more than one sink is an error naming both candidates.
Matching is a pure function over an injected sink list, so it is testable without
an audio server.

`ErrDeviceNotFound` from a PulseAudio attempt aborts the fallback chain: on a
machine whose sound server already owns the hardware, silently opening raw ALSA
would either fail with `EBUSY` or steal the device.

`OutputDevices()` lists PulseAudio sinks (ID = sink name, Name = description)
when the server is reachable, then ALSA playback PCMs from
`snd_device_name_hint`, skipping `Input`, `null`, and `default` entries as
`deviceCandidates` does. If a stack fails, its error is dropped when the other
stack produced devices; when no device was found at all, the joined errors are
returned. On platforms without device selection it returns
`ErrDeviceSelectionUnsupported`.

## Lifecycle and concurrency

- `multipleContextsSupported` becomes a per-platform constant: true for the Unix
  backend, false elsewhere. The `contextCreated` guard and its mutex are used only
  where multiple contexts are unsupported.
- Each context owns its mux, `pulse.Client`/`PlaybackStream`, or ALSA PCM handle.
  Nothing mutable is shared between contexts.
- `loadALSA()` currently documents that it needs no synchronization because a
  context is created at most once. It becomes `sync.Once` plus a cached error,
  because two concurrent `NewContext` calls would otherwise race on the purego
  bindings.
- Two contexts on the same ALSA PCM need `dmix`/`plug` (`default`,
  `plug:hw:X`). A bare `hw:` PCM is exclusive and surfaces `EBUSY` as an
  initialization error. This is documented, not worked around.
- Suspend/Resume/Err remain per context. There is no process-wide suspend.

## Testing

- Pure table tests (no audio server, package `oto`): sink matching (exact ID,
  case-insensitive ID, description match, ambiguous description, no match),
  ALSA candidate expansion, resolution table for every `Backend`/`DeviceName`
  combination, `Backend.String`.
- Hardware test gated on `OTO_TEST_DEVICE_2`: two contexts bound to different
  devices initialize, both report no error after playing silence, and a bogus
  `DeviceName` yields `ErrDeviceNotFound` from `Context.Err`.
- CI only vets and cross-builds on Linux (`go test` is skipped there for #259),
  so the hardware test is run locally against the real sinks/devices before the
  branch is considered done.

## Documentation

README: the "only one context" statements, the Linux section, and the option
list; `Context`, `NewContext`, and `NewContextOptions` doc comments.
