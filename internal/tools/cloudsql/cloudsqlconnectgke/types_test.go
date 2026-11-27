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

package cloudsqlconnectgke

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/util/cloudsqlconnect"
	sqladmin "google.golang.org/api/sqladmin/v1"
)

func TestExtractSQLInfo(t *testing.T) {
	inst := &sqladmin.DatabaseInstance{
		DatabaseVersion:          "POSTGRES_16",
		DnsName:                  "abc.us-central1.sql.goog.",
		PscServiceAttachmentLink: "projects/x/regions/us-central1/serviceAttachments/a",
		Settings: &sqladmin.Settings{
			IpConfiguration: &sqladmin.IpConfiguration{
				SslMode:   "ENCRYPTED_ONLY",
				PscConfig: &sqladmin.PscConfig{PscEnabled: true, AllowedConsumerProjects: []string{"app"}},
			},
			DatabaseFlags: []*sqladmin.DatabaseFlags{{Name: "cloudsql.iam_authentication", Value: "on"}},
		},
	}
	got := extractSQLInfo(inst)
	if !got.PSCEnabled || got.DNSName != inst.DnsName || got.PSCServiceAttachment != inst.PscServiceAttachmentLink {
		t.Errorf("PSC fields not extracted: %+v", got)
	}
	if diff := cmp.Diff([]string{"app"}, got.PSCAllowedConsumers); diff != "" {
		t.Errorf("allowed consumers mismatch: %s", diff)
	}
	if got.SSLMode != "ENCRYPTED_ONLY" {
		t.Errorf("SSLMode = %q", got.SSLMode)
	}
	if !got.IAMDatabaseAuthEnabled {
		t.Error("IAM auth flag not detected")
	}

	inst.Settings.DatabaseFlags = []*sqladmin.DatabaseFlags{{Name: "cloudsql_iam_authentication", Value: "off"}}
	if extractSQLInfo(inst).IAMDatabaseAuthEnabled {
		t.Error("IAM auth reported on when flag is off")
	}
}

func TestConnectResultJSONShape(t *testing.T) {
	res := &connectResult{
		Summary:       "ok",
		ConnectResult: &cloudsqlconnect.ConnectResult{ComputeType: cloudsqlconnect.ComputeGKE},
		EnvironmentConfig: environmentConfig{
			EnvironmentConfig: cloudsqlconnect.EnvironmentConfig{SidecarYAML: "sidecar"},
			DeploymentYAML:    "deployment",
		},
		Troubleshooting: []troubleshootingTip{{Symptom: "s"}},
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["summary"] != "ok" || got["computeType"] != "gke" || got["troubleshooting"] == nil {
		t.Errorf("missing top-level fields: %s", raw)
	}
	env, ok := got["environmentConfig"].(map[string]any)
	if !ok || env["sidecarYaml"] != "sidecar" || env["deploymentYaml"] != "deployment" {
		t.Errorf("environmentConfig should carry shared and GKE manifests: %s", raw)
	}
}
