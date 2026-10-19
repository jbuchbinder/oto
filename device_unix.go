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
	"fmt"
	"strings"

	"github.com/jfreymuth/pulse"
)

// deviceTarget is one attempt at opening an output device.
type deviceTarget struct {
	// backend is the backend that should open the device.
	backend Backend

	// device is a PulseAudio sink name, or an ALSA PCM name. Empty means the
	// backend's default device.
	device string
}

// pulseSinkInfo is the part of a PulseAudio sink that device selection needs.
type pulseSinkInfo struct {
	id   string
	name string
}

// listALSAOutputDevices enumerates ALSA playback devices. driver_alsa_unix.go's
// init sets it on the platforms where that file is built; it is nil elsewhere.
var listALSAOutputDevices func() ([]OutputDevice, error)

// resolveOutputDevice returns the attempts for a device selection, in order.
//
// With BackendAuto and an explicit name, ALSA is only reached when PulseAudio is
// unreachable: a name that the reachable sound server does not know is an error,
// because opening raw ALSA while the server owns the hardware either fails with
// EBUSY or steals the device.
func resolveOutputDevice(deviceName string, backend Backend) ([]deviceTarget, error) {
	switch backend {
	case BackendAuto:
		if deviceName == "" {
			return []deviceTarget{{backend: BackendPulseAudio}, {backend: BackendALSA}}, nil
		}
		return []deviceTarget{
			{backend: BackendPulseAudio, device: deviceName},
			{backend: BackendALSA, device: deviceName},
		}, nil
	case BackendPulseAudio:
		return []deviceTarget{{backend: BackendPulseAudio, device: deviceName}}, nil
	case BackendALSA:
		return []deviceTarget{{backend: BackendALSA, device: deviceName}}, nil
	default:
		return nil, fmt.Errorf("oto: unknown backend: %d", int(backend))
	}
}

// deviceNotFoundHint explains how to select an ALSA device when the failed
// attempt was the PulseAudio one in an automatic selection.
func deviceNotFoundHint(err error, cfg contextConfig) error {
	if cfg.backend == BackendAuto && cfg.deviceName != "" {
		return fmt.Errorf("%w (to use an ALSA PCM name such as \"hw:1,0\", set Backend to BackendALSA)", err)
	}
	return err
}

// matchPulseSink returns the index of the sink that name refers to.
//
// An exact sink ID match wins. Otherwise the name is compared case-insensitively
// with sink IDs and then with sink descriptions. A description shared by more
// than one sink is an error rather than an arbitrary pick.
func matchPulseSink(sinks []pulseSinkInfo, name string) (int, error) {
	for i, s := range sinks {
		if s.id == name {
			return i, nil
		}
	}

	matched := matchSinkIndexes(sinks, name, func(s pulseSinkInfo) string { return s.id })
	if len(matched) == 0 {
		matched = matchSinkIndexes(sinks, name, func(s pulseSinkInfo) string { return s.name })
	}

	switch len(matched) {
	case 1:
		return matched[0], nil
	case 0:
		return -1, fmt.Errorf("%w: no PulseAudio sink matches %q; available sinks: %s", ErrDeviceNotFound, name, pulseSinkList(sinks))
	default:
		descriptions := make([]string, len(matched))
		for i, index := range matched {
			descriptions[i] = pulseSinkDescription(sinks[index])
		}
		return -1, fmt.Errorf("%w: %q matches multiple PulseAudio sinks: %s", ErrDeviceNotFound, name, strings.Join(descriptions, ", "))
	}
}

func matchSinkIndexes(sinks []pulseSinkInfo, name string, key func(pulseSinkInfo) string) []int {
	var matched []int
	for i, s := range sinks {
		if key(s) != "" && strings.EqualFold(key(s), name) {
			matched = append(matched, i)
		}
	}
	return matched
}

func pulseSinkDescription(s pulseSinkInfo) string {
	if s.name == "" {
		return fmt.Sprintf("%q", s.id)
	}
	return fmt.Sprintf("%q (%s)", s.id, s.name)
}

func pulseSinkList(sinks []pulseSinkInfo) string {
	if len(sinks) == 0 {
		return "none"
	}
	descriptions := make([]string, len(sinks))
	for i, s := range sinks {
		descriptions[i] = pulseSinkDescription(s)
	}
	return strings.Join(descriptions, ", ")
}

// alsaNameCandidates expands an explicit ALSA PCM name into the names to try.
// The plug plugin is needed because Oto always writes 32-bit floats.
func alsaNameCandidates(name string) []string {
	if name == "" {
		return nil
	}
	if strings.HasPrefix(name, "plug:") || strings.HasPrefix(name, "plughw:") {
		return []string{name}
	}
	return []string{name, "plug:" + name}
}

// OutputDevices returns the output devices that can be selected with
// NewContextOptions.DeviceName.
//
// On Linux and BSD, PulseAudio sinks are listed first, followed by ALSA playback
// devices. Listing a backend that is not available is not an error as long as
// another backend reported devices; when nothing could be listed, the errors are
// returned joined.
//
// On platforms without device selection, OutputDevices returns
// ErrDeviceSelectionUnsupported.
func OutputDevices() ([]OutputDevice, error) {
	var devices []OutputDevice
	var errs []error

	if client, err := pulse.NewClient(pulse.ClientApplicationName("Oto")); err != nil {
		errs = append(errs, fmt.Errorf("oto: PulseAudio device enumeration failed: %w", err))
	} else {
		sinks, err := client.ListSinks()
		client.Close()
		if err != nil {
			errs = append(errs, fmt.Errorf("oto: PulseAudio device enumeration failed: %w", err))
		} else {
			for _, sink := range sinks {
				devices = append(devices, OutputDevice{
					ID:      sink.ID(),
					Name:    sink.Name(),
					Backend: BackendPulseAudio,
				})
			}
		}
	}

	if listALSAOutputDevices == nil {
		errs = append(errs, errors.New("oto: ALSA device enumeration is not available on this platform"))
	} else if alsaDevices, err := listALSAOutputDevices(); err != nil {
		errs = append(errs, err)
	} else {
		devices = append(devices, alsaDevices...)
	}

	if len(devices) == 0 {
		if len(errs) == 0 {
			return nil, nil
		}
		return nil, errors.Join(errs...)
	}
	return devices, nil
}
