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

func TestValidateGCEConnection(t *testing.T) {
	tcs := []struct {
		desc          string
		sqlInfo       *CloudSQLInstanceInfo
		vmInfo        *GCEInstanceInfo
		wantValid     bool
		wantMinChecks int
	}{
		{
			desc: "valid with private IP and same VPC",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: true,
				PrivateIPAddress: "10.0.0.5",
				VPCNetwork:       "projects/my-project/global/networks/default",
			},
			vmInfo: &GCEInstanceInfo{
				VPCNetwork:     "default",
				ServiceAccount: "my-sa@my-project.iam.gserviceaccount.com",
			},
			wantValid:     true,
			wantMinChecks: 3,
		},
		{
			desc: "valid with public IP only",
			sqlInfo: &CloudSQLInstanceInfo{
				PublicIPEnabled:  true,
				PublicIPAddress:  "35.1.2.3",
				PrivateIPEnabled: false,
			},
			vmInfo: &GCEInstanceInfo{
				HasExternalIP: true,
			},
			wantValid:     true,
			wantMinChecks: 2,
		},
		{
			desc: "invalid with no IP addresses",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: false,
				PublicIPEnabled:  false,
			},
			vmInfo:        &GCEInstanceInfo{},
			wantValid:     false,
			wantMinChecks: 2,
		},
		{
			desc: "different VPCs adds warning",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: true,
				PrivateIPAddress: "10.0.0.5",
				VPCNetwork:       "projects/my-project/global/networks/vpc-a",
			},
			vmInfo: &GCEInstanceInfo{
				VPCNetwork: "vpc-b",
			},
			wantValid:     true,
			wantMinChecks: 2,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			result := ValidateGCEConnection(tc.sqlInfo, tc.vmInfo)

			if result.Valid != tc.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tc.wantValid)
			}

			if len(result.Checks) < tc.wantMinChecks {
				t.Errorf("got %d checks, want at least %d", len(result.Checks), tc.wantMinChecks)
			}

			// Verify all checks have required fields
			for _, check := range result.Checks {
				if check.Name == "" {
					t.Error("check Name should not be empty")
				}
				if check.Status == "" {
					t.Error("check Status should not be empty")
				}
				if check.Message == "" {
					t.Error("check Message should not be empty")
				}
			}
		})
	}
}

func TestValidateGKEConnection(t *testing.T) {
	tcs := []struct {
		desc          string
		sqlInfo       *CloudSQLInstanceInfo
		gkeInfo       *GKEClusterInfo
		wantValid     bool
		wantMinChecks int
	}{
		{
			desc: "valid with workload identity and VPC native",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: true,
				PrivateIPAddress: "10.0.0.5",
				VPCNetwork:       "projects/my-project/global/networks/default",
			},
			gkeInfo: &GKEClusterInfo{
				WorkloadIdentity: true,
				VPCNative:        true,
				VPCNetwork:       "default",
			},
			wantValid:     true,
			wantMinChecks: 4,
		},
		{
			desc: "valid without workload identity (shows warning)",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: true,
				PrivateIPAddress: "10.0.0.5",
			},
			gkeInfo: &GKEClusterInfo{
				WorkloadIdentity: false,
				VPCNative:        true,
			},
			wantValid:     true,
			wantMinChecks: 3,
		},
		{
			desc: "private cluster info captured",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: true,
			},
			gkeInfo: &GKEClusterInfo{
				WorkloadIdentity: true,
				PrivateCluster:   true,
			},
			wantValid:     true,
			wantMinChecks: 3,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			result := ValidateGKEConnection(tc.sqlInfo, tc.gkeInfo)

			if result.Valid != tc.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tc.wantValid)
			}

			if len(result.Checks) < tc.wantMinChecks {
				t.Errorf("got %d checks, want at least %d", len(result.Checks), tc.wantMinChecks)
			}
		})
	}
}

func TestValidateCloudRunConnection(t *testing.T) {
	tcs := []struct {
		desc          string
		sqlInfo       *CloudSQLInstanceInfo
		runInfo       *CloudRunServiceInfo
		wantValid     bool
		wantMinChecks int
	}{
		{
			desc: "valid Cloud Run connection",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: true,
				ConnectionName:   "project:region:instance",
			},
			runInfo: &CloudRunServiceInfo{
				Name:           "my-service",
				ServiceAccount: "my-sa@my-project.iam.gserviceaccount.com",
			},
			wantValid:     true,
			wantMinChecks: 2,
		},
		{
			desc: "with existing Cloud SQL connection",
			sqlInfo: &CloudSQLInstanceInfo{
				ConnectionName: "project:region:instance",
			},
			runInfo: &CloudRunServiceInfo{
				Name:              "my-service",
				CloudSQLInstances: []string{"project:region:instance"},
			},
			wantValid:     true,
			wantMinChecks: 2,
		},
		{
			desc: "with VPC connector",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: true,
			},
			runInfo: &CloudRunServiceInfo{
				Name:         "my-service",
				VPCConnector: "projects/p/locations/us-central1/connectors/conn",
			},
			wantValid:     true,
			wantMinChecks: 2,
		},
		{
			desc: "with direct VPC egress",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: true,
			},
			runInfo: &CloudRunServiceInfo{
				Name:            "my-service",
				DirectVPCEgress: true,
			},
			wantValid:     true,
			wantMinChecks: 2,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			result := ValidateCloudRunConnection(tc.sqlInfo, tc.runInfo)

			if result.Valid != tc.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tc.wantValid)
			}

			if len(result.Checks) < tc.wantMinChecks {
				t.Errorf("got %d checks, want at least %d", len(result.Checks), tc.wantMinChecks)
			}

			// Cloud Run should always pass built-in support check
			found := false
			for _, check := range result.Checks {
				if check.Name == "Cloud Run Cloud SQL Integration" {
					found = true
					if check.Status != "pass" {
						t.Errorf("Cloud Run integration check status = %q, want 'pass'", check.Status)
					}
				}
			}
			if !found {
				t.Error("expected Cloud Run Cloud SQL Integration check")
			}
		})
	}
}

func TestValidateLocalConnection(t *testing.T) {
	tcs := []struct {
		desc          string
		sqlInfo       *CloudSQLInstanceInfo
		wantValid     bool
		wantMinChecks int
	}{
		{
			desc: "valid with public IP",
			sqlInfo: &CloudSQLInstanceInfo{
				PublicIPEnabled: true,
				PublicIPAddress: "35.1.2.3",
			},
			wantValid:     true,
			wantMinChecks: 2,
		},
		{
			desc: "warning with private IP only",
			sqlInfo: &CloudSQLInstanceInfo{
				PrivateIPEnabled: true,
				PrivateIPAddress: "10.0.0.5",
				PublicIPEnabled:  false,
			},
			wantValid:     true,
			wantMinChecks: 2,
		},
		{
			desc: "invalid with no IPs",
			sqlInfo: &CloudSQLInstanceInfo{
				PublicIPEnabled:  false,
				PrivateIPEnabled: false,
			},
			wantValid:     false,
			wantMinChecks: 2,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			result := ValidateLocalConnection(tc.sqlInfo)

			if result.Valid != tc.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tc.wantValid)
			}

			if len(result.Checks) < tc.wantMinChecks {
				t.Errorf("got %d checks, want at least %d", len(result.Checks), tc.wantMinChecks)
			}

			// Local should always have recommendations
			if len(result.Recommendations) == 0 {
				t.Error("expected recommendations for local connection")
			}
		})
	}
}

func TestExtractNetworkName(t *testing.T) {
	tcs := []struct {
		input string
		want  string
	}{
		{
			input: "projects/my-project/global/networks/default",
			want:  "default",
		},
		{
			input: "projects/my-project/global/networks/my-vpc",
			want:  "my-vpc",
		},
		{
			input: "default",
			want:  "default",
		},
		{
			input: "",
			want:  "",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.input, func(t *testing.T) {
			got := ExtractNetworkName(tc.input)
			if got != tc.want {
				t.Errorf("ExtractNetworkName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidationCheckStatuses(t *testing.T) {
	// Verify that validation functions use valid status values
	validStatuses := map[string]bool{
		"pass": true,
		"fail": true,
		"warn": true,
		"info": true,
	}

	// Test GCE validation
	sqlInfo := &CloudSQLInstanceInfo{
		PrivateIPEnabled: true,
		PublicIPEnabled:  true,
		VPCNetwork:       "default",
	}
	vmInfo := &GCEInstanceInfo{VPCNetwork: "default"}
	gceResult := ValidateGCEConnection(sqlInfo, vmInfo)

	for _, check := range gceResult.Checks {
		if !validStatuses[check.Status] {
			t.Errorf("invalid status %q in GCE validation check %q", check.Status, check.Name)
		}
	}

	// Test GKE validation
	gkeInfo := &GKEClusterInfo{WorkloadIdentity: true}
	gkeResult := ValidateGKEConnection(sqlInfo, gkeInfo)

	for _, check := range gkeResult.Checks {
		if !validStatuses[check.Status] {
			t.Errorf("invalid status %q in GKE validation check %q", check.Status, check.Name)
		}
	}

	// Test Cloud Run validation
	runInfo := &CloudRunServiceInfo{Name: "test"}
	runResult := ValidateCloudRunConnection(sqlInfo, runInfo)

	for _, check := range runResult.Checks {
		if !validStatuses[check.Status] {
			t.Errorf("invalid status %q in Cloud Run validation check %q", check.Status, check.Name)
		}
	}

	// Test Local validation
	localResult := ValidateLocalConnection(sqlInfo)

	for _, check := range localResult.Checks {
		if !validStatuses[check.Status] {
			t.Errorf("invalid status %q in Local validation check %q", check.Status, check.Name)
		}
	}
}
