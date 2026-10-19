// Copyright 2026 The Oto Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !android && !darwin && !js && !windows && !nintendosdk && !playstation5

package oto

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestBackendString(t *testing.T) {
	tests := []struct {
		backend Backend
		want    string
	}{
		{BackendAuto, "auto"},
		{BackendPulseAudio, "pulseaudio"},
		{BackendALSA, "alsa"},
		{Backend(-1), "Backend(-1)"},
		{Backend(42), "Backend(42)"},
	}
	for _, tt := range tests {
		if got := tt.backend.String(); got != tt.want {
			t.Errorf("Backend(%d).String() = %q, want %q", int(tt.backend), got, tt.want)
		}
	}
}

func testPulseSinks() []pulseSinkInfo {
	return []pulseSinkInfo{
		{id: "alsa_output.pci-0000_00_1f.3.analog-stereo", name: "Built-in Audio Analog Stereo"},
		{id: "alsa_output.usb-Focusrite.analog-surround-40", name: "Focusrite USB"},
		{id: "alsa_output.pci-0000_00_1f.3.hdmi-stereo", name: "Built-in Audio Digital Stereo (HDMI)"},
		{id: "sink.duplicate.one", name: "Line Out"},
		{id: "sink.duplicate.two", name: "line out"},
	}
}

func TestMatchPulseSink(t *testing.T) {
	tests := []struct {
		name      string
		sinks     []pulseSinkInfo
		query     string
		wantIndex int
		wantErr   bool
		errHas    []string
	}{
		{
			name:      "exact id",
			query:     "alsa_output.usb-Focusrite.analog-surround-40",
			wantIndex: 1,
		},
		{
			name:      "case-insensitive id",
			query:     "ALSA_OUTPUT.PCI-0000_00_1F.3.HDMI-STEREO",
			wantIndex: 2,
		},
		{
			name:      "exact description",
			query:     "Focusrite USB",
			wantIndex: 1,
		},
		{
			name:      "case-insensitive description",
			query:     "built-in audio analog stereo",
			wantIndex: 0,
		},
		{
			name:    "not found",
			query:   "no such sink",
			wantErr: true,
			errHas:  []string{"available sinks", "alsa_output.pci-0000_00_1f.3.analog-stereo"},
		},
		{
			name:    "ambiguous description",
			query:   "LINE OUT",
			wantErr: true,
			errHas:  []string{"multiple PulseAudio sinks", "sink.duplicate.one", "sink.duplicate.two"},
		},
		{
			name:    "empty sink list",
			sinks:   []pulseSinkInfo{},
			query:   "anything",
			wantErr: true,
			errHas:  []string{"none"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sinks := tt.sinks
			if sinks == nil {
				sinks = testPulseSinks()
			}
			index, err := matchPulseSink(sinks, tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("matchPulseSink(%q) = %d without an error", tt.query, index)
				}
				if !errors.Is(err, ErrDeviceNotFound) {
					t.Errorf("matchPulseSink(%q) error = %v, want it to wrap ErrDeviceNotFound", tt.query, err)
				}
				for _, want := range tt.errHas {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("matchPulseSink(%q) error = %q, want it to contain %q", tt.query, err.Error(), want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("matchPulseSink(%q) failed: %v", tt.query, err)
			}
			if index != tt.wantIndex {
				t.Errorf("matchPulseSink(%q) = %d, want %d", tt.query, index, tt.wantIndex)
			}
		})
	}
}

func TestAlsaNameCandidates(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "empty uses the scan", input: ""},
		{name: "hw", input: "hw:1,0", want: []string{"hw:1,0", "plug:hw:1,0"}},
		{name: "default", input: "default", want: []string{"default", "plug:default"}},
		{name: "already plugged", input: "plug:hw:1,0", want: []string{"plug:hw:1,0"}},
		{name: "plughw", input: "plughw:1,0", want: []string{"plughw:1,0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := alsaNameCandidates(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("alsaNameCandidates(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveOutputDevice(t *testing.T) {
	tests := []struct {
		name    string
		device  string
		backend Backend
		want    []deviceTarget
		wantErr bool
	}{
		{
			name:    "auto default",
			backend: BackendAuto,
			want:    []deviceTarget{{backend: BackendPulseAudio}, {backend: BackendALSA}},
		},
		{
			name:    "pulse default",
			backend: BackendPulseAudio,
			want:    []deviceTarget{{backend: BackendPulseAudio}},
		},
		{
			name:    "alsa default",
			backend: BackendALSA,
			want:    []deviceTarget{{backend: BackendALSA}},
		},
		{
			name:    "pulse device",
			device:  "alsa_output.usb-Focusrite.analog-surround-40",
			backend: BackendPulseAudio,
			want:    []deviceTarget{{backend: BackendPulseAudio, device: "alsa_output.usb-Focusrite.analog-surround-40"}},
		},
		{
			name:    "alsa device",
			device:  "hw:1,0",
			backend: BackendALSA,
			want:    []deviceTarget{{backend: BackendALSA, device: "hw:1,0"}},
		},
		{
			name:    "auto device keeps the fallback",
			device:  "hw:1,0",
			backend: BackendAuto,
			want: []deviceTarget{
				{backend: BackendPulseAudio, device: "hw:1,0"},
				{backend: BackendALSA, device: "hw:1,0"},
			},
		},
		{
			name:    "unknown backend",
			backend: Backend(42),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveOutputDevice(tt.device, tt.backend)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveOutputDevice(%q, %s) = %v without an error", tt.device, tt.backend, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveOutputDevice(%q, %s) failed: %v", tt.device, tt.backend, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resolveOutputDevice(%q, %s) = %v, want %v", tt.device, tt.backend, got, tt.want)
			}
		})
	}
}

func TestDeviceNotFoundHint(t *testing.T) {
	notFound := errors.Join(ErrDeviceNotFound)

	hinted := deviceNotFoundHint(notFound, contextConfig{backend: BackendAuto, deviceName: "hw:1,0"})
	if !errors.Is(hinted, ErrDeviceNotFound) {
		t.Errorf("deviceNotFoundHint lost ErrDeviceNotFound: %v", hinted)
	}
	if !strings.Contains(hinted.Error(), "BackendALSA") {
		t.Errorf("deviceNotFoundHint = %q, want a hint about BackendALSA", hinted.Error())
	}

	plain := deviceNotFoundHint(notFound, contextConfig{backend: BackendPulseAudio, deviceName: "hw:1,0"})
	if plain.Error() != notFound.Error() {
		t.Errorf("deviceNotFoundHint added a hint for an explicit backend: %q", plain.Error())
	}
}

func TestOutputDevices(t *testing.T) {
	devices, err := OutputDevices()
	if err != nil {
		t.Skipf("no output devices on this machine: %v", err)
	}
	if len(devices) == 0 {
		t.Skip("no output devices on this machine")
	}
	for _, device := range devices {
		if device.ID == "" {
			t.Errorf("OutputDevices returned a device with an empty ID: %+v", device)
		}
		switch device.Backend {
		case BackendPulseAudio, BackendALSA:
		default:
			t.Errorf("OutputDevices returned a device with an unexpected backend: %+v", device)
		}
	}
}

func TestNewContextRejectsUnknownBackend(t *testing.T) {
	options := &NewContextOptions{
		SampleRate:   48000,
		ChannelCount: 2,
		Backend:      Backend(42),
	}
	if _, _, err := NewContext(options); err == nil {
		t.Fatal("NewContext accepted an unknown backend")
	}
}
