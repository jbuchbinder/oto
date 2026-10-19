// Copyright 2021 The Oto Authors
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

	"github.com/ebitengine/oto/v3/internal/mux"
)

// unixBackend is the part of a context that talks to the actual audio device.
type unixBackend interface {
	Suspend() error
	Resume() error
	Err() error
}

// newALSAContext creates the ALSA fallback backend. driver_alsa_unix.go's init sets it on the
// platforms where that file is built; it is nil elsewhere (e.g. OpenBSD), in which case only the
// PulseAudio backend is attempted.
var newALSAContext func(sampleRate, channelCount int, mux *mux.Mux, bufferSizeInBytes int, deviceName string) (unixBackend, error)

type context struct {
	mux     *mux.Mux
	backend unixBackend

	ready chan struct{}
	err   atomicError
}

func newContext(cfg contextConfig) (*context, chan struct{}, error) {
	ctx := &context{
		mux:   mux.New(cfg.sampleRate, cfg.channelCount, cfg.format),
		ready: make(chan struct{}),
	}

	targets, err := resolveOutputDevice(cfg.deviceName, cfg.backend)
	if err != nil {
		ctx.err.Join(err)
		close(ctx.ready)
		return ctx, ctx.ready, nil
	}

	// Initializing a driver might take some time, so do it asynchronously.
	// PulseAudio is the default; if no server is reachable, fall back to ALSA.
	go func() {
		defer close(ctx.ready)

		var errs []error
		for _, target := range targets {
			switch target.backend {
			case BackendPulseAudio:
				pc, err := newPulseContext(cfg.sampleRate, cfg.channelCount, ctx.mux, cfg.bufferSizeInBytes, cfg.applicationName, target.device)
				if err == nil {
					ctx.backend = pc
					return
				}
				if errors.Is(err, ErrDeviceNotFound) {
					// The name was resolved against the sound server, so do not fall
					// back to raw ALSA while PulseAudio owns the hardware.
					ctx.err.Join(deviceNotFoundHint(err, cfg))
					return
				}
				errs = append(errs, fmt.Errorf("PulseAudio: %w", err))
			case BackendALSA:
				if newALSAContext == nil {
					errs = append(errs, errors.New("ALSA: backend is not available on this platform"))
					continue
				}
				ac, err := newALSAContext(cfg.sampleRate, cfg.channelCount, ctx.mux, cfg.bufferSizeInBytes, target.device)
				if err == nil {
					ctx.backend = ac
					return
				}
				errs = append(errs, fmt.Errorf("ALSA: %w", err))
			}
		}

		ctx.err.Join(fmt.Errorf("oto: initialization failed: %w", errors.Join(errs...)))
	}()

	return ctx, ctx.ready, nil
}

func (c *context) Suspend() error {
	<-c.ready
	if c.backend != nil {
		return c.backend.Suspend()
	}
	return nil
}

func (c *context) Resume() error {
	<-c.ready
	if c.backend != nil {
		return c.backend.Resume()
	}
	return nil
}

func (c *context) Err() error {
	if err := c.err.Load(); err != nil {
		return err
	}

	select {
	case <-c.ready:
	default:
		return nil
	}

	if c.backend != nil {
		return c.backend.Err()
	}
	return nil
}
