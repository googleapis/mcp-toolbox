// Copyright 2025 Google LLC
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

package cloudhealthcare

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlCloudHealthcare(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-instance
			type: cloud-healthcare
			project: my-project
			region: us-central1
			dataset: my-dataset
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": Config{
					Name:           "my-instance",
					Type:           SourceType,
					Project:        "my-project",
					Region:         "us-central1",
					Dataset:        "my-dataset",
					UseClientOAuth: false,
				},
			},
		},
		{
			desc: "use client auth example",
			in: `
			kind: source
			name: my-instance
			type: cloud-healthcare
			project: my-project
			region: us
			dataset: my-dataset
			useClientOAuth: true
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": Config{
					Name:           "my-instance",
					Type:           SourceType,
					Project:        "my-project",
					Region:         "us",
					Dataset:        "my-dataset",
					UseClientOAuth: true,
				},
			},
		},
		{
			desc: "with allowed stores example",
			in: `
			kind: source
			name: my-instance
			type: cloud-healthcare
			project: my-project
			region: us
			dataset: my-dataset
			allowedFhirStores:
				- my-fhir-store
			allowedDicomStores:
				- my-dicom-store1
				- my-dicom-store2
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": Config{
					Name:               "my-instance",
					Type:               SourceType,
					Project:            "my-project",
					Region:             "us",
					Dataset:            "my-dataset",
					AllowedFHIRStores:  []string{"my-fhir-store"},
					AllowedDICOMStores: []string{"my-dicom-store1", "my-dicom-store2"},
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if !cmp.Equal(tc.want, got) {
				t.Fatalf("incorrect parse: want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestFailParseFromYaml(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "extra field",
			in: `
			kind: source
			name: my-instance
			type: cloud-healthcare
			project: my-project
			region: us-central1
			dataset: my-dataset
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-instance\" as \"cloud-healthcare\": [2:1] unknown field \"foo\"\n   1 | dataset: my-dataset\n>  2 | foo: bar\n       ^\n   3 | name: my-instance\n   4 | project: my-project\n   5 | region: us-central1\n   6 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-instance
			type: cloud-healthcare
			project: my-project
			region: us-central1
			`,
			err: "error unmarshaling source: unable to parse source \"my-instance\" as \"cloud-healthcare\": Key: 'Config.Dataset' Error:Field validation for 'Dataset' failed on the 'required' tag",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if errStr != tc.err {
				t.Fatalf("unexpected error: got %q, want %q", errStr, tc.err)
			}
		})
	}
}

func TestValidateFHIRPageURL(t *testing.T) {
	src := &Source{
		Config: Config{
			Project:           "my-project",
			Region:            "us-central1",
			Dataset:           "my-dataset",
			AllowedFHIRStores: []string{"store1", "store2"},
		},
		allowedFHIRStores: map[string]struct{}{
			"store1": {},
			"store2": {},
		},
	}

	tests := []struct {
		desc    string
		pageURL string
		wantErr bool
		wantURL string
	}{
		{
			desc:    "Valid URL v1",
			pageURL: "https://healthcare.googleapis.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
			wantErr: false,
			wantURL: "https://healthcare.googleapis.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
		},
		{
			desc:    "Valid URL mTLS",
			pageURL: "https://healthcare.mtls.googleapis.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
			wantErr: false,
			wantURL: "https://healthcare.mtls.googleapis.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
		},
		{
			desc:    "Valid URL v1beta1",
			pageURL: "https://healthcare.googleapis.com/v1beta1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store2/fhir/Patient?_count=10",
			wantErr: false,
			wantURL: "https://healthcare.googleapis.com/v1beta1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store2/fhir/Patient?_count=10",
		},
		{
			desc:    "Valid URL with port",
			pageURL: "https://healthcare.googleapis.com:443/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
			wantErr: false,
			wantURL: "https://healthcare.googleapis.com:443/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
		},
		{
			desc:    "Valid URL with trailing slash",
			pageURL: "https://healthcare.googleapis.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient/",
			wantErr: false,
			wantURL: "https://healthcare.googleapis.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
		},
		{
			desc:    "Invalid scheme",
			pageURL: "http://healthcare.googleapis.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
			wantErr: true,
		},
		{
			desc:    "Invalid version v0",
			pageURL: "https://healthcare.googleapis.com/v0/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
			wantErr: true,
		},
		{
			desc:    "Invalid version v1.0",
			pageURL: "https://healthcare.googleapis.com/v1.0/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
			wantErr: true,
		},
		{
			desc:    "Missing version prefix (projects as part 0)",
			pageURL: "https://healthcare.googleapis.com/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
			wantErr: true,
		},
		{
			desc:    "Valid version v2",
			pageURL: "https://healthcare.googleapis.com/v2/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
			wantErr: false,
			wantURL: "https://healthcare.googleapis.com/v2/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
		},
		{
			desc:    "Invalid host",
			pageURL: "https://evil.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
			wantErr: true,
		},
		{
			desc:    "Host suffix spoofing attempt",
			pageURL: "https://healthcare.googleapis.com.evil.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
			wantErr: true,
		},
		{
			desc:    "Authority userinfo spoofing attempt",
			pageURL: "https://healthcare.googleapis.com@evil.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
			wantErr: true,
		},
		{
			desc:    "Invalid project",
			pageURL: "https://healthcare.googleapis.com/v1/projects/other-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
			wantErr: true,
		},
		{
			desc:    "Invalid location",
			pageURL: "https://healthcare.googleapis.com/v1/projects/my-project/locations/us-east1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
			wantErr: true,
		},
		{
			desc:    "Invalid dataset",
			pageURL: "https://healthcare.googleapis.com/v1/projects/my-project/locations/us-central1/datasets/other-dataset/fhirStores/store1/fhir/Patient",
			wantErr: true,
		},
		{
			desc:    "Disallowed FHIR store",
			pageURL: "https://healthcare.googleapis.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store3/fhir/Patient",
			wantErr: true,
		},
		{
			desc:    "Directory traversal attack path bypass attempt 1",
			pageURL: "https://healthcare.googleapis.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/../../../../something",
			wantErr: true,
		},
		{
			desc:    "Directory traversal attack path bypass attempt 2",
			pageURL: "https://healthcare.googleapis.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient/../../../../something",
			wantErr: true,
		},
		{
			desc:    "Too short path",
			pageURL: "https://healthcare.googleapis.com/v1/projects/my-project",
			wantErr: true,
		},
		{
			desc:    "Non-FHIR endpoint on same host",
			pageURL: "https://healthcare.googleapis.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/dicomStores/store1",
			wantErr: true,
		},
		{
			desc:    "Valid URL with uppercase host and scheme",
			pageURL: "HTTPS://HEALTHCARE.GOOGLEAPIS.COM/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
			wantErr: false,
			wantURL: "https://healthcare.googleapis.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
		},
		{
			desc:    "Invalid URL without version prefix",
			pageURL: "https://healthcare.googleapis.com/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/store1/fhir/Patient",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			gotURL, err := src.validateFHIRPageURL(tc.pageURL)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateFHIRPageURL(%q) err = %v, wantErr = %v", tc.pageURL, err, tc.wantErr)
			}
			if err == nil && gotURL != tc.wantURL {
				t.Errorf("validateFHIRPageURL(%q) gotURL = %q, wantURL = %q", tc.pageURL, gotURL, tc.wantURL)
			}
		})
	}
}

func TestValidateFHIRPageURLNoAllowedStores(t *testing.T) {
	// If s.Config.AllowedFHIRStores is empty (meaning all FHIR stores are allowed),
	// validateFHIRPageURL should allow any fhir store ID.
	src := &Source{
		Config: Config{
			Project: "my-project",
			Region:  "us-central1",
			Dataset: "my-dataset",
		},
	}

	tests := []struct {
		desc    string
		pageURL string
		wantErr bool
		wantURL string
	}{
		{
			desc:    "Valid URL with storeX",
			pageURL: "https://healthcare.googleapis.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/storeX/fhir/Patient",
			wantErr: false,
			wantURL: "https://healthcare.googleapis.com/v1/projects/my-project/locations/us-central1/datasets/my-dataset/fhirStores/storeX/fhir/Patient",
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			gotURL, err := src.validateFHIRPageURL(tc.pageURL)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateFHIRPageURL(%q) err = %v, wantErr = %v", tc.pageURL, err, tc.wantErr)
			}
			if err == nil && gotURL != tc.wantURL {
				t.Errorf("validateFHIRPageURL(%q) gotURL = %q, wantURL = %q", tc.pageURL, gotURL, tc.wantURL)
			}
		})
	}
}
