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

package oto_test

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/oto/v3"
)

// TestSecondOutputDevice proves that a second context can play through a device
// other than the default one while the first context keeps running.
//
// It needs a real device, so it only runs when OTO_TEST_DEVICE_2 is set to an ID
// from OutputDevices. OTO_TEST_DEVICE_2_BACKEND selects the backend
// ("auto", "pulseaudio", or "alsa") and defaults to "auto".
func TestSecondOutputDevice(t *testing.T) {
	deviceName := os.Getenv("OTO_TEST_DEVICE_2")
	if deviceName == "" {
		t.Skip("set OTO_TEST_DEVICE_2 to a device ID from OutputDevices to run this test")
	}

	var backend oto.Backend
	switch strings.ToLower(os.Getenv("OTO_TEST_DEVICE_2_BACKEND")) {
	case "", "auto":
		backend = oto.BackendAuto
	case "pulseaudio":
		backend = oto.BackendPulseAudio
	case "alsa":
		backend = oto.BackendALSA
	default:
		t.Fatalf("unknown OTO_TEST_DEVICE_2_BACKEND: %q", os.Getenv("OTO_TEST_DEVICE_2_BACKEND"))
	}

	options := &oto.NewContextOptions{
		SampleRate:   48000,
		ChannelCount: 2,
		Format:       oto.FormatFloat32LE,
		DeviceName:   deviceName,
		Backend:      backend,
	}
	second, ready, err := oto.NewContext(options)
	if err != nil {
		t.Fatalf("NewContext(%q, %s) failed: %v", deviceName, backend, err)
	}
	<-ready
	if err := second.Err(); err != nil {
		t.Fatalf("context on %q failed to initialize: %v", deviceName, err)
	}

	// Play silence through both contexts at the same time.
	silence := make([]byte, 48000*2*4)
	firstPlayer := theContext.NewPlayer(bytes.NewReader(silence))
	secondPlayer := second.NewPlayer(bytes.NewReader(silence))
	firstPlayer.Play()
	secondPlayer.Play()
	time.Sleep(200 * time.Millisecond)

	if !firstPlayer.IsPlaying() {
		t.Error("the default context stopped playing")
	}
	if !secondPlayer.IsPlaying() {
		t.Errorf("the context on %q stopped playing", deviceName)
	}
	firstPlayer.Pause()
	secondPlayer.Pause()

	if err := theContext.Err(); err != nil {
		t.Errorf("the default context reported an error: %v", err)
	}
	if err := second.Err(); err != nil {
		t.Errorf("the context on %q reported an error: %v", deviceName, err)
	}
}

// TestUnknownDeviceName checks that a name no sink matches is reported as
// ErrDeviceNotFound instead of silently falling back to another device. It
// needs a reachable PulseAudio server, so it skips on ALSA-only machines.
func TestUnknownDeviceName(t *testing.T) {
	probeOptions := &oto.NewContextOptions{
		SampleRate:   48000,
		ChannelCount: 2,
		Format:       oto.FormatFloat32LE,
		Backend:      oto.BackendPulseAudio,
	}
	probe, ready, err := oto.NewContext(probeOptions)
	if err != nil {
		t.Fatalf("NewContext for the PulseAudio probe failed: %v", err)
	}
	<-ready
	if err := probe.Err(); err != nil {
		t.Skipf("no reachable PulseAudio server: %v", err)
	}

	options := &oto.NewContextOptions{
		SampleRate:   48000,
		ChannelCount: 2,
		Format:       oto.FormatFloat32LE,
		DeviceName:   "oto-definitely-not-a-device",
		Backend:      oto.BackendPulseAudio,
	}
	ctx, ready, err := oto.NewContext(options)
	if err != nil {
		t.Fatalf("NewContext failed: %v", err)
	}
	<-ready
	if err := ctx.Err(); !errors.Is(err, oto.ErrDeviceNotFound) {
		t.Fatalf("Context.Err() = %v, want ErrDeviceNotFound", err)
	}
}
