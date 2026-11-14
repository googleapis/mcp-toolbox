// Copyright 2026 Google LLC
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

package server

import "time"

// DefaultHTTPMaxRequestBytes is the default max size (in bytes) for MCP HTTP request bodies.
const DefaultHTTPMaxRequestBytes int64 = 10 << 20

// DefaultReadHeaderTimeout is the maximum time allowed to receive the complete
// HTTP request header from a client. A short deadline prevents Slowloris
// attacks where an attacker trickles headers to hold connections open
// indefinitely without consuming server resources for legitimate requests.
const DefaultReadHeaderTimeout = 10 * time.Second
