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

//go:build android || darwin || js || windows || nintendosdk || playstation5

package oto

// These drivers hold a package-level context pointer for their platform
// callbacks, or have no concept of output devices, so only one context is
// possible and the device cannot be chosen.
const multipleContextsSupported = false

const deviceSelectionSupported = false
