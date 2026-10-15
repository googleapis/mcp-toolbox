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
	"fmt"

	"github.com/googleapis/mcp-toolbox/internal/util/cloudsqlconnect"
)

// getGKERecommendations returns connection recommendations for a GKE
// cluster. The Auth Proxy sidecar is always primary: it is the approach
// Cloud SQL documents for GKE, works for every engine, and pairs with
// Workload Identity for keyless auth. Its IP mode follows
// resolveNetworkPath. Alternatives only appear when they would work:
// the Connector libraries are skipped for SQL Server (the generated code
// paths cover Postgres and cloudsqlconnect.MySQL), and direct private IP requires a
// VPC-native cluster in the instance's VPC.
func getGKERecommendations(sqlInfo *instanceInfo, gkeInfo *clusterInfo) (cloudsqlconnect.ConnectionRecommendation, []cloudsqlconnect.ConnectionRecommendation) {
	path := resolveNetworkPath(sqlInfo, gkeInfo)

	identityReq := "Workload Identity enabled on the cluster, with a Kubernetes service account bound to a Google service account"
	if gkeInfo.Autopilot {
		identityReq = "Kubernetes service account bound to a Google service account (Workload Identity is always on in Autopilot)"
	}

	primary := cloudsqlconnect.ConnectionRecommendation{
		Method:      cloudsqlconnect.MethodAuthProxy,
		Name:        fmt.Sprintf("%s sidecar (%s)", cloudsqlconnect.MethodNameAuthProxy, pathShortLabel(path)),
		Description: "Run the Auth Proxy as a sidecar in each pod; the app connects to 127.0.0.1 while the proxy handles TLS and IAM authorization",
		Priority:    1,
		Security:    "very high",
		Complexity:  "medium",
		Performance: "excellent",
		Requirements: []string{
			identityReq,
			"Google service account has roles/cloudsql.client in the instance's project",
		},
	}
	switch path {
	case pathPrivateIP:
		primary.Requirements = append(primary.Requirements, "Proxy started with --private-ip")
	case pathPublicIP:
		if gkeInfo.PrivateCluster {
			primary.Requirements = append(primary.Requirements, "Cloud NAT for the private nodes' network and region")
		}
		primary.Considerations = append(primary.Considerations, "Traffic leaves the VPC but is encrypted with mTLS by the proxy; no authorized networks needed")
	case pathPSC:
		primary.Requirements = append(primary.Requirements, "PSC endpoint and DNS record in the cluster's VPC", "Proxy started with --psc")
	case pathNone:
		primary.Considerations = append(primary.Considerations, "Blocked until a network path exists; see validation issues")
	}
	if nativeSidecarHint(gkeInfo) {
		primary.Considerations = append(primary.Considerations, "Runs as a native sidecar, so the app starts only after the proxy is ready and Jobs complete cleanly")
	} else {
		primary.Considerations = append(primary.Considerations, "Cluster is older than 1.29: classic sidecar, so the app should retry its first connection and Jobs need --quitquitquit")
	}
	if sqlInfo.IAMDatabaseAuthEnabled {
		primary.Considerations = append(primary.Considerations, "IAM database auth is enabled: add --auto-iam-authn to log in as the Google service account without a password (the database user must be that IAM user)")
	}

	var alternatives []cloudsqlconnect.ConnectionRecommendation
	if sqlInfo.DatabaseType != cloudsqlconnect.SQLServer {
		alt := cloudsqlconnect.ConnectionRecommendation{
			Method:      cloudsqlconnect.MethodConnector,
			Name:        cloudsqlconnect.MethodNameConnector,
			Description: "Connect from application code with the Cloud SQL language connector and Workload Identity; no sidecar",
			Priority:    2,
			Security:    "very high",
			Complexity:  "low",
			Performance: "excellent",
			Requirements: []string{
				"Application uses Python, Java, Go, or Node.js",
				identityReq,
			},
		}
		if path == pathPrivateIP {
			alt.Considerations = []string{"Set the connector's IP type to PRIVATE"}
		}
		alternatives = append(alternatives, alt)
	}

	if privateIPReachable(sqlInfo, gkeInfo) {
		alt := cloudsqlconnect.ConnectionRecommendation{
			Method:      cloudsqlconnect.MethodDirectPrivateIP,
			Name:        cloudsqlconnect.MethodNameDirectPrivateIP,
			Description: "Connect pods straight to the instance's private IP",
			Priority:    len(alternatives) + 2,
			Security:    "high",
			Complexity:  "low",
			Performance: "optimal",
			Requirements: []string{
				"VPC-native cluster in the instance's VPC",
				"Egress firewall allows the database port to the instance",
			},
			Considerations: []string{
				"No IAM authorization: anyone in the VPC with the password can connect",
			},
		}
		if sqlInfo.RequireSSL || (sqlInfo.SSLMode != "" && sqlInfo.SSLMode != "ALLOW_UNENCRYPTED_AND_ENCRYPTED") {
			alt.Considerations = append(alt.Considerations, "Instance enforces SSL: configure the driver for TLS (and client certificates if required)")
		}
		alternatives = append(alternatives, alt)
	}

	return primary, alternatives
}

// nativeSidecarHint reports whether the cluster can run native sidecars.
func nativeSidecarHint(gkeInfo *clusterInfo) bool {
	return supportsNativeSidecar(gkeInfo.MasterVersion)
}

func pathShortLabel(path networkPath) string {
	switch path {
	case pathPrivateIP:
		return "Private IP"
	case pathPublicIP:
		return "Public IP"
	case pathPSC:
		return "Private Service Connect"
	default:
		return "no network path"
	}
}

// GetCloudRunRecommendations returns connection recommendations for Cloud Run.
