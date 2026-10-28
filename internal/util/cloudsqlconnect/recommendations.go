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

// GetGCERecommendations returns connection recommendations for GCE VM.
// It analyzes the network configuration and returns the best connection method
// as primary, along with alternatives.
func GetGCERecommendations(sqlInfo *CloudSQLInstanceInfo, vmInfo *GCEInstanceInfo, sameVPC bool) (ConnectionRecommendation, []ConnectionRecommendation) {
	var primary ConnectionRecommendation
	var alternatives []ConnectionRecommendation

	// If same VPC and private IP enabled, recommend direct connection
	if sameVPC && sqlInfo.PrivateIPEnabled {
		primary = ConnectionRecommendation{
			Method:      MethodDirectPrivateIP,
			Name:        MethodNameDirectPrivateIP,
			Description: "Connect directly to Cloud SQL using its private IP address",
			Priority:    1,
			Security:    "high",
			Complexity:  "low",
			Performance: "optimal",
			Requirements: []string{
				"VM and Cloud SQL in same VPC network",
				"Private IP enabled on Cloud SQL",
				"Firewall allows egress to Cloud SQL",
			},
		}

		alternatives = append(alternatives, ConnectionRecommendation{
			Method:      MethodAuthProxy,
			Name:        MethodNameAuthProxy + " (Private IP)",
			Description: "Use Auth Proxy for IAM-based authentication over private IP",
			Priority:    2,
			Security:    "very high",
			Complexity:  "medium",
			Performance: "excellent",
			Requirements: []string{
				"Cloud SQL Auth Proxy installed",
				"Service account with Cloud SQL Client role",
			},
		})
	} else if sqlInfo.PrivateIPEnabled {
		// Different VPC but private IP available
		primary = ConnectionRecommendation{
			Method:      MethodAuthProxy,
			Name:        MethodNameAuthProxy + " (Private IP)",
			Description: "Use Auth Proxy with private IP for secure connections",
			Priority:    1,
			Security:    "very high",
			Complexity:  "medium",
			Performance: "excellent",
			Requirements: []string{
				"Cloud SQL Auth Proxy installed",
				"Service account with Cloud SQL Client role",
				"VPC peering or shared VPC for private IP access",
			},
			Considerations: []string{
				"Requires VPC peering if VMs are in different VPC",
			},
		}
	} else if sqlInfo.PublicIPEnabled {
		primary = ConnectionRecommendation{
			Method:      MethodAuthProxy,
			Name:        MethodNameAuthProxy + " (Public IP)",
			Description: "Use Auth Proxy with public IP",
			Priority:    1,
			Security:    "high",
			Complexity:  "medium",
			Performance: "good",
			Requirements: []string{
				"Cloud SQL Auth Proxy installed",
				"Service account with Cloud SQL Client role",
				"VM has internet access",
			},
			Considerations: []string{
				"Connection traverses public internet (encrypted)",
			},
		}
	}

	// Always add connector library as alternative
	alternatives = append(alternatives, ConnectionRecommendation{
		Method:      MethodConnector,
		Name:        MethodNameConnector,
		Description: "Use language-specific connector for automatic secure connections",
		Priority:    3,
		Security:    "very high",
		Complexity:  "low",
		Performance: "excellent",
		Requirements: []string{
			"Application uses supported language (Python, Java, Go, Node.js)",
			"Service account with Cloud SQL Client role",
		},
	})

	return primary, alternatives
}

// GetGKERecommendations returns connection recommendations for GKE cluster.
func GetGKERecommendations(sqlInfo *CloudSQLInstanceInfo, gkeInfo *GKEClusterInfo, sameVPC bool) (ConnectionRecommendation, []ConnectionRecommendation) {
	var primary ConnectionRecommendation
	var alternatives []ConnectionRecommendation

	// Primary recommendation: Auth Proxy sidecar with Workload Identity
	primary = ConnectionRecommendation{
		Method:      MethodAuthProxy,
		Name:        MethodNameAuthProxy + " Sidecar with Workload Identity",
		Description: "Deploy Auth Proxy as sidecar container with Workload Identity for secure, keyless authentication",
		Priority:    1,
		Security:    "very high",
		Complexity:  "medium",
		Performance: "excellent",
		Requirements: []string{
			"Workload Identity enabled on GKE cluster",
			"Kubernetes Service Account linked to GCP Service Account",
			"GCP Service Account has Cloud SQL Client role",
		},
	}

	// Alternative: Connector library
	alternatives = append(alternatives, ConnectionRecommendation{
		Method:      MethodConnector,
		Name:        MethodNameConnector,
		Description: "Use connector library with Workload Identity - no sidecar needed",
		Priority:    2,
		Security:    "very high",
		Complexity:  "low",
		Performance: "excellent",
		Requirements: []string{
			"Workload Identity enabled",
			"Application uses supported language",
		},
	})

	// Alternative: Direct private IP if same VPC
	if sameVPC && sqlInfo.PrivateIPEnabled && gkeInfo.VPCNative {
		alternatives = append(alternatives, ConnectionRecommendation{
			Method:      MethodDirectPrivateIP,
			Name:        MethodNameDirectPrivateIP,
			Description: "Connect directly using private IP (VPC-native cluster required)",
			Priority:    3,
			Security:    "high",
			Complexity:  "low",
			Performance: "optimal",
			Requirements: []string{
				"GKE cluster is VPC-native",
				"Same VPC as Cloud SQL",
				"Private IP enabled on Cloud SQL",
			},
			Considerations: []string{
				"No automatic IAM authentication",
				"Requires managing database credentials separately",
			},
		})
	}

	return primary, alternatives
}

// GetCloudRunRecommendations returns connection recommendations for Cloud Run.
func GetCloudRunRecommendations(sqlInfo *CloudSQLInstanceInfo, runInfo *CloudRunServiceInfo) (ConnectionRecommendation, []ConnectionRecommendation) {
	var primary ConnectionRecommendation
	var alternatives []ConnectionRecommendation

	// Primary recommendation: Built-in Unix socket
	primary = ConnectionRecommendation{
		Method:      MethodUnixSocket,
		Name:        MethodNameUnixSocket,
		Description: "Use Cloud Run's native Cloud SQL integration via Unix socket",
		Priority:    1,
		Security:    "very high",
		Complexity:  "very low",
		Performance: "excellent",
		Requirements: []string{
			"Cloud Run service account has Cloud SQL Client role",
			"Cloud SQL connection added to service configuration",
		},
	}

	// Alternative: Connector library
	alternatives = append(alternatives, ConnectionRecommendation{
		Method:      MethodConnector,
		Name:        MethodNameConnector,
		Description: "Use connector library for programmatic connections",
		Priority:    2,
		Security:    "very high",
		Complexity:  "low",
		Performance: "excellent",
		Requirements: []string{
			"Application uses supported language",
			"Service account has Cloud SQL Client role",
		},
	})

	// Alternative: Private IP via VPC
	if sqlInfo.PrivateIPEnabled {
		alternatives = append(alternatives, ConnectionRecommendation{
			Method:      MethodDirectPrivateIP,
			Name:        MethodNameDirectPrivateIP + " via VPC Connector",
			Description: "Connect using private IP through Serverless VPC Access",
			Priority:    3,
			Security:    "high",
			Complexity:  "medium",
			Performance: "good",
			Requirements: []string{
				"Serverless VPC Access Connector configured",
				"Private IP enabled on Cloud SQL",
			},
		})
	}

	return primary, alternatives
}

// GetLocalRecommendations returns connection recommendations for local development.
func GetLocalRecommendations(sqlInfo *CloudSQLInstanceInfo) (ConnectionRecommendation, []ConnectionRecommendation) {
	var primary ConnectionRecommendation
	var alternatives []ConnectionRecommendation

	// Primary recommendation: Auth Proxy
	primary = ConnectionRecommendation{
		Method:      MethodAuthProxy,
		Name:        MethodNameAuthProxy,
		Description: "Use Cloud SQL Auth Proxy for secure local development",
		Priority:    1,
		Security:    "very high",
		Complexity:  "low",
		Performance: "good",
		Requirements: []string{
			"Cloud SQL Auth Proxy installed locally",
			"Application Default Credentials configured",
			"User has Cloud SQL Client role",
		},
	}

	// Alternative: Connector library
	alternatives = append(alternatives, ConnectionRecommendation{
		Method:      MethodConnector,
		Name:        MethodNameConnector,
		Description: "Use connector library - no separate proxy process needed",
		Priority:    2,
		Security:    "very high",
		Complexity:  "very low",
		Performance: "good",
		Requirements: []string{
			"Application uses supported language (Python, Java, Go, Node.js)",
			"Application Default Credentials configured",
			"User has Cloud SQL Client role",
		},
	})

	return primary, alternatives
}
