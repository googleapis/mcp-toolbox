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
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/util/cloudsqlconnect"
	sqladmin "google.golang.org/api/sqladmin/v1"
)

// The types below extend the shared cloudsqlconnect types with the facts
// only the GKE tool reasons over. They embed the shared types instead of
// changing them, so the GCE tool and shared package are untouched.

// instanceInfo is the shared Cloud SQL instance view plus the Private
// Service Connect, SSL and IAM authentication settings that decide how
// pods connect.
type instanceInfo struct {
	*cloudsqlconnect.CloudSQLInstanceInfo
	SSLMode                string
	PSCEnabled             bool
	PSCServiceAttachment   string
	PSCAllowedConsumers    []string
	DNSName                string
	IAMDatabaseAuthEnabled bool
}

// extractSQLInfo reuses the shared extractor and adds the GKE-only
// fields.
func extractSQLInfo(inst *sqladmin.DatabaseInstance) *instanceInfo {
	info := &instanceInfo{CloudSQLInstanceInfo: cloudsqlconnect.ExtractSQLInfo(inst)}
	if inst.Settings == nil {
		return info
	}
	if ipc := inst.Settings.IpConfiguration; ipc != nil {
		info.SSLMode = ipc.SslMode
		if ipc.PscConfig != nil && ipc.PscConfig.PscEnabled {
			info.PSCEnabled = true
			info.PSCAllowedConsumers = ipc.PscConfig.AllowedConsumerProjects
			info.PSCServiceAttachment = inst.PscServiceAttachmentLink
			info.DNSName = inst.DnsName
		}
	}
	info.IAMDatabaseAuthEnabled = hasIAMAuthFlag(inst.Settings.DatabaseFlags)
	return info
}

// hasIAMAuthFlag reports whether IAM database authentication is switched
// on. Postgres spells the flag "cloudsql.iam_authentication"; MySQL spells
// it "cloudsql_iam_authentication". SQL Server has no equivalent.
func hasIAMAuthFlag(flags []*sqladmin.DatabaseFlags) bool {
	for _, f := range flags {
		if f == nil || !strings.EqualFold(f.Value, "on") {
			continue
		}
		if f.Name == "cloudsql.iam_authentication" || f.Name == "cloudsql_iam_authentication" {
			return true
		}
	}
	return false
}

// clusterInfo contains the GKE cluster facts the tool reasons over.
type clusterInfo struct {
	Name     string
	Location string
	Project  string
	// VPCNetwork is the short network name (e.g. "default").
	VPCNetwork string
	// NetworkURI is the fully-qualified network
	// ("projects/HOST/global/networks/NAME"). For Shared VPC clusters the
	// project is the host project, which is what must match Cloud SQL.
	NetworkURI       string
	Subnetwork       string
	WorkloadIdentity bool
	WorkloadPool     string
	PrivateCluster   bool
	VPCNative        bool
	Autopilot        bool
	Status           string
	MasterVersion    string
	// NodePoolsWithoutGKEMetadata lists Standard-cluster node pools whose
	// workload metadata mode is not GKE_METADATA. Pods scheduled there
	// silently fall back to the node's service account even when
	// Workload Identity is enabled on the cluster.
	NodePoolsWithoutGKEMetadata []string
}

// environmentConfig is the shared environment config plus the complete
// Kubernetes manifests the setup steps reference.
type environmentConfig struct {
	cloudsqlconnect.EnvironmentConfig
	ServiceAccountYAML string `json:"serviceAccountYaml,omitempty"`
	DeploymentYAML     string `json:"deploymentYaml,omitempty"`
}

// troubleshootingTip maps a symptom the user is likely to hit after
// following the setup steps to its most common cause and fix, so an agent
// can match error text from logs without re-deriving the diagnosis.
type troubleshootingTip struct {
	Symptom string `json:"symptom"`
	Cause   string `json:"cause"`
	Fix     string `json:"fix"`
}

// connectResult is the shared ConnectResult with a leading summary, GKE
// manifests, and troubleshooting tips. Its EnvironmentConfig field
// shadows the embedded one (encoding/json prefers the shallower field),
// so the JSON keeps the same "environmentConfig" key as the GCE tool.
type connectResult struct {
	// Summary is a one-paragraph, plain-language verdict an agent can
	// relay to the user before walking through setupSteps.
	Summary string `json:"summary"`
	*cloudsqlconnect.ConnectResult
	EnvironmentConfig environmentConfig    `json:"environmentConfig"`
	Troubleshooting   []troubleshootingTip `json:"troubleshooting"`
}
