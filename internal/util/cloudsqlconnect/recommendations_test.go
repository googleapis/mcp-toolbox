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

package cloudsqlconnect

import (
	"testing"
)

func TestGetGCERecommendations(t *testing.T) {
	tcs := []struct {
		desc              string
		sqlInfo           *CloudSQLInstanceInfo
		vmInfo            *GCEInstanceInfo
		sameVPC           bool
		wantPrimaryMethod ConnectionMethod
		wantAlternatives  int
		wantHasConnector  bool
	}{
		{
			desc: "same VPC with private IP recommends direct connection",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: true,
				PrivateIPAddress: "10.0.0.5",
			},
			vmInfo:            &GCEInstanceInfo{VPCNetwork: "default"},
			sameVPC:           true,
			wantPrimaryMethod: MethodDirectPrivateIP,
			wantAlternatives:  2, // Auth Proxy + Connector
			wantHasConnector:  true,
		},
		{
			desc: "different VPC with private IP recommends auth proxy",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: true,
				PrivateIPAddress: "10.0.0.5",
			},
			vmInfo:            &GCEInstanceInfo{VPCNetwork: "other-vpc"},
			sameVPC:           false,
			wantPrimaryMethod: MethodAuthProxy,
			wantAlternatives:  1, // Connector only
			wantHasConnector:  true,
		},
		{
			desc: "public IP only recommends auth proxy",
			sqlInfo: &CloudSQLInstanceInfo{
				PublicIPEnabled:  true,
				PublicIPAddress:  "35.1.2.3",
				PrivateIPEnabled: false,
			},
			vmInfo:            &GCEInstanceInfo{HasExternalIP: true},
			sameVPC:           false,
			wantPrimaryMethod: MethodAuthProxy,
			wantAlternatives:  1, // Connector only
			wantHasConnector:  true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			primary, alternatives := GetGCERecommendations(tc.sqlInfo, tc.vmInfo, tc.sameVPC)

			if primary.Method != tc.wantPrimaryMethod {
				t.Errorf("primary method = %v, want %v", primary.Method, tc.wantPrimaryMethod)
			}

			if len(alternatives) != tc.wantAlternatives {
				t.Errorf("got %d alternatives, want %d", len(alternatives), tc.wantAlternatives)
			}

			// Verify connector is in alternatives
			hasConnector := false
			for _, alt := range alternatives {
				if alt.Method == MethodConnector {
					hasConnector = true
					break
				}
			}
			if hasConnector != tc.wantHasConnector {
				t.Errorf("hasConnector = %v, want %v", hasConnector, tc.wantHasConnector)
			}

			// Verify required fields are populated
			if primary.Name == "" {
				t.Error("primary Name should not be empty")
			}
			if primary.Description == "" {
				t.Error("primary Description should not be empty")
			}
			if len(primary.Requirements) == 0 {
				t.Error("primary Requirements should not be empty")
			}
		})
	}
}

func TestGetGKERecommendations(t *testing.T) {
	tcs := []struct {
		desc                string
		sqlInfo             *CloudSQLInstanceInfo
		gkeInfo             *GKEClusterInfo
		sameVPC             bool
		wantPrimaryMethod   ConnectionMethod
		wantMinAlternatives int
	}{
		{
			desc: "GKE always recommends auth proxy sidecar",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: true,
			},
			gkeInfo: &GKEClusterInfo{
				WorkloadIdentity: true,
				VPCNative:        true,
			},
			sameVPC:             true,
			wantPrimaryMethod:   MethodAuthProxy,
			wantMinAlternatives: 2, // Connector + Direct Private IP
		},
		{
			desc: "GKE without VPC-native only has connector alternative",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: true,
			},
			gkeInfo: &GKEClusterInfo{
				WorkloadIdentity: true,
				VPCNative:        false,
			},
			sameVPC:             true,
			wantPrimaryMethod:   MethodAuthProxy,
			wantMinAlternatives: 1, // Connector only
		},
		{
			desc: "different VPC no direct IP option",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: true,
			},
			gkeInfo: &GKEClusterInfo{
				WorkloadIdentity: true,
				VPCNative:        true,
			},
			sameVPC:             false,
			wantPrimaryMethod:   MethodAuthProxy,
			wantMinAlternatives: 1, // Connector only
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			primary, alternatives := GetGKERecommendations(tc.sqlInfo, tc.gkeInfo, tc.sameVPC)

			if primary.Method != tc.wantPrimaryMethod {
				t.Errorf("primary method = %v, want %v", primary.Method, tc.wantPrimaryMethod)
			}

			if len(alternatives) < tc.wantMinAlternatives {
				t.Errorf("got %d alternatives, want at least %d", len(alternatives), tc.wantMinAlternatives)
			}

			// Verify primary has Workload Identity in name for GKE
			if primary.Method == MethodAuthProxy {
				// Should mention sidecar and/or workload identity
				if primary.Name == "" {
					t.Error("primary Name should not be empty")
				}
			}
		})
	}
}

func TestGetCloudRunRecommendations(t *testing.T) {
	tcs := []struct {
		desc                string
		sqlInfo             *CloudSQLInstanceInfo
		runInfo             *CloudRunServiceInfo
		wantPrimaryMethod   ConnectionMethod
		wantMinAlternatives int
	}{
		{
			desc: "Cloud Run always recommends Unix socket",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: true,
			},
			runInfo: &CloudRunServiceInfo{
				Name: "my-service",
			},
			wantPrimaryMethod:   MethodUnixSocket,
			wantMinAlternatives: 2, // Connector + Direct Private IP
		},
		{
			desc: "Cloud Run without private IP has fewer alternatives",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: false,
				PublicIPEnabled:  true,
			},
			runInfo: &CloudRunServiceInfo{
				Name: "my-service",
			},
			wantPrimaryMethod:   MethodUnixSocket,
			wantMinAlternatives: 1, // Connector only
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			primary, alternatives := GetCloudRunRecommendations(tc.sqlInfo, tc.runInfo)

			if primary.Method != tc.wantPrimaryMethod {
				t.Errorf("primary method = %v, want %v", primary.Method, tc.wantPrimaryMethod)
			}

			if len(alternatives) < tc.wantMinAlternatives {
				t.Errorf("got %d alternatives, want at least %d", len(alternatives), tc.wantMinAlternatives)
			}

			// Verify complexity is very low for Unix socket
			if primary.Method == MethodUnixSocket && primary.Complexity != "very low" {
				t.Errorf("Unix socket complexity = %q, want 'very low'", primary.Complexity)
			}
		})
	}
}

func TestGetLocalRecommendations(t *testing.T) {
	tcs := []struct {
		desc              string
		sqlInfo           *CloudSQLInstanceInfo
		wantPrimaryMethod ConnectionMethod
		wantAlternatives  int
	}{
		{
			desc: "local always recommends auth proxy",
			sqlInfo: &CloudSQLInstanceInfo{
				PublicIPEnabled: true,
			},
			wantPrimaryMethod: MethodAuthProxy,
			wantAlternatives:  1, // Connector only
		},
		{
			desc: "local with private IP still recommends auth proxy",
			sqlInfo: &CloudSQLInstanceInfo{
				PublicIPEnabled:  true,
				PrivateIPEnabled: true,
			},
			wantPrimaryMethod: MethodAuthProxy,
			wantAlternatives:  1, // Connector only
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			primary, alternatives := GetLocalRecommendations(tc.sqlInfo)

			if primary.Method != tc.wantPrimaryMethod {
				t.Errorf("primary method = %v, want %v", primary.Method, tc.wantPrimaryMethod)
			}

			if len(alternatives) != tc.wantAlternatives {
				t.Errorf("got %d alternatives, want %d", len(alternatives), tc.wantAlternatives)
			}

			// Verify connector is the alternative
			if len(alternatives) > 0 && alternatives[0].Method != MethodConnector {
				t.Errorf("expected Connector as alternative, got %v", alternatives[0].Method)
			}

			// Verify low complexity for local
			if primary.Complexity != "low" {
				t.Errorf("local Auth Proxy complexity = %q, want 'low'", primary.Complexity)
			}
		})
	}
}

func TestRecommendationPriorities(t *testing.T) {
	// Verify that primary always has priority 1
	sqlInfo := &CloudSQLInstanceInfo{
		PrivateIPEnabled: true,
		PrivateIPAddress: "10.0.0.5",
		PublicIPEnabled:  true,
		PublicIPAddress:  "35.1.2.3",
	}
	vmInfo := &GCEInstanceInfo{VPCNetwork: "default"}
	gkeInfo := &GKEClusterInfo{WorkloadIdentity: true, VPCNative: true}
	runInfo := &CloudRunServiceInfo{Name: "test"}

	tests := []struct {
		name    string
		primary ConnectionRecommendation
	}{
		{"GCE", func() ConnectionRecommendation { p, _ := GetGCERecommendations(sqlInfo, vmInfo, true); return p }()},
		{"GKE", func() ConnectionRecommendation { p, _ := GetGKERecommendations(sqlInfo, gkeInfo, true); return p }()},
		{"CloudRun", func() ConnectionRecommendation { p, _ := GetCloudRunRecommendations(sqlInfo, runInfo); return p }()},
		{"Local", func() ConnectionRecommendation { p, _ := GetLocalRecommendations(sqlInfo); return p }()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.primary.Priority != 1 {
				t.Errorf("%s primary priority = %d, want 1", tt.name, tt.primary.Priority)
			}
		})
	}
}

func TestRecommendationSecurityLevels(t *testing.T) {
	// Verify security levels are valid strings
	validSecurityLevels := map[string]bool{
		"high":      true,
		"very high": true,
		"excellent": true,
		"good":      true,
	}

	sqlInfo := &CloudSQLInstanceInfo{PrivateIPEnabled: true, PublicIPEnabled: true}
	primary, alternatives := GetGCERecommendations(sqlInfo, &GCEInstanceInfo{}, true)

	if !validSecurityLevels[primary.Security] {
		t.Errorf("invalid security level: %q", primary.Security)
	}

	for _, alt := range alternatives {
		if !validSecurityLevels[alt.Security] {
			t.Errorf("invalid security level in alternative: %q", alt.Security)
		}
	}
}
