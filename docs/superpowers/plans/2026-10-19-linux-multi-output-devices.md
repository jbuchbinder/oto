# Multiple output devices on Linux — implementation plan

**Goal:** let several `oto.Context` instances coexist on Unix, each bound to a
chosen output device, and expose device discovery.

**Architecture:** all name/backend resolution lives in `device_unix.go` as pure,
table-testable functions; drivers open resolved targets. The single-context gate
becomes a per-platform constant. ALSA library loading becomes `sync.Once`.

**Tech stack:** Go 1.25, purego (libasound), `github.com/jfreymuth/pulse`.

**Spec:** `docs/superpowers/specs/2026-10-19-linux-multi-output-devices-design.md`

## Global constraints

- Public API additions only; no existing field or method changes.
- Non-Unix platforms return `ErrDeviceSelectionUnsupported` for any device
  option and keep exactly one context per process.
- Empty `DeviceName` with `BackendAuto` must reproduce today's behavior.
- No new module dependencies.
- CI: `go vet ./...` and cross-builds must stay green; `go test` runs locally
  only for hardware tests.

## Review focus

- `DeviceName` set on macOS/Windows: must fail at `NewContext`, not be silently
  ignored.
- `BackendALSA` on OpenBSD (no libasound): must report the backend as
  unavailable, not panic on a nil function.
- Two contexts created concurrently: ALSA binding must not race.
- `DeviceName` naming an ALSA PCM while PulseAudio is running: must not silently
  fall back to raw ALSA.
- Descriptions that are not unique: must error, not pick the first sink.

---

### Task 1: Public types, validation, and platform constants

**Files:** `context.go`, `platform_unix.go` (new), `platform_other.go` (new),
all `driver_*.go` `newContext` call sites.

**Produces:** `Backend` (+`String`), `NewContextOptions.DeviceName`/`Backend`,
`OutputDevice`, `ErrDeviceSelectionUnsupported`, `ErrDeviceNotFound`,
`contextConfig`, `multipleContextsSupported`, `deviceSelectionSupported`.

- Write tests: invalid backend value is rejected; `Backend.String` values.
- Implement the types and validation; change `newContext` to take
  `contextConfig`; non-Unix drivers ignore the device fields.
- Verify: `go build ./...`, `GOOS=windows go build ./...`, `GOOS=darwin go build ./...`.

### Task 2: Pure resolution helpers (TDD)

**Files:** `device_unix_test.go` (new), `device_unix.go` (new).

- Tests for `matchPulseSink` (exact ID, case-insensitive ID, description,
  ambiguity, not found), `alsaNameCandidates`, and `resolveOutputDevice` for all
  six `Backend`/`DeviceName` combinations.
- Implement until green.

### Task 3: PulseAudio device targeting

**Files:** `driver_pulseaudio_unix.go`.

- `newPulseContext` takes `deviceName`; non-empty resolves via `ListSinks` +
  `matchPulseSink` and appends `pulse.PlaybackSink`.
- Verify: build + `go vet`.

### Task 4: ALSA device targeting, `sync.Once`, enumeration hook

**Files:** `driver_alsa_unix.go`.

- `newALSAContext` takes `deviceName`; explicit names use `alsaNameCandidates`
  instead of the full scan.
- Replace `loadALSA` with `sync.Once` + cached error.
- Add `alsaOutputDevices()` and the `listALSAOutputDevices` hook.

### Task 5: `OutputDevices()`

**Files:** `device_unix.go`, `platform_other.go`.

- Unix: PulseAudio sinks then ALSA PCMs; drop per-stack errors when devices were
  found; joined error otherwise.
- Other platforms: return `ErrDeviceSelectionUnsupported`.

### Task 6: Multi-context on Unix

**Files:** `context.go`.

- Guard `contextCreated` only when `!multipleContextsSupported`.
- Audit: no other package-level mutable state in the Unix path.

### Task 7: Documentation and hardware verification

**Files:** `README.md`, doc comments.

- README: per-platform context rule, device-selection section, two-context
  snippet.
- Hardware probe: create two contexts against distinct devices and confirm both
  report no error and play; confirm a bogus name yields `ErrDeviceNotFound`.

## Verification gates

- `go build ./...`, `go vet ./...`
- `GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=darwin GOARCH=arm64 go build ./...`,
  `GOOS=js GOARCH=wasm go build ./...`
- `go test -run 'Backend|PulseSink|AlsaName|ResolveOutput' .`
- Local hardware probe with `OTO_TEST_DEVICE_2` when two devices are available.
