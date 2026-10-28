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
	"fmt"
)

// GenerateGCESetupSteps generates setup steps for GCE VM connection.
func GenerateGCESetupSteps(method ConnectionMethod, connectionName string, port int, privateIP string, serviceAccount string) []SetupStep {
	steps := []SetupStep{
		{
			Order:       1,
			Title:       "Enable Cloud SQL Admin API",
			Description: "Ensure the Cloud SQL Admin API is enabled in your project",
			Command:     "gcloud services enable sqladmin.googleapis.com",
		},
		{
			Order:       2,
			Title:       "Grant IAM permissions",
			Description: fmt.Sprintf("Grant Cloud SQL Client role to VM service account: %s", serviceAccount),
			Command:     fmt.Sprintf("gcloud projects add-iam-policy-binding PROJECT_ID --member='serviceAccount:%s' --role='roles/cloudsql.client'", serviceAccount),
		},
	}

	switch method {
	case MethodAuthProxy:
		steps = append(steps,
			SetupStep{
				Order:       3,
				Title:       "Install Cloud SQL Auth Proxy",
				Description: "Download and install the Cloud SQL Auth Proxy on your VM",
				Command:     "curl -o cloud-sql-proxy https://storage.googleapis.com/cloud-sql-connectors/cloud-sql-proxy/v2.8.0/cloud-sql-proxy.linux.amd64 && chmod +x cloud-sql-proxy",
			},
			SetupStep{
				Order:       4,
				Title:       "Start Auth Proxy",
				Description: "Start the Auth Proxy to create a secure tunnel",
				Command:     fmt.Sprintf("./cloud-sql-proxy %s &", connectionName),
			},
			SetupStep{
				Order:       5,
				Title:       "Connect to database",
				Description: fmt.Sprintf("Connect your application to localhost:%d", port),
			},
		)
	case MethodDirectPrivateIP:
		steps = append(steps,
			SetupStep{
				Order:       3,
				Title:       "Verify VPC connectivity",
				Description: "Ensure VM can reach Cloud SQL private IP",
				Command:     fmt.Sprintf("ping -c 3 %s", privateIP),
			},
			SetupStep{
				Order:       4,
				Title:       "Connect to database",
				Description: fmt.Sprintf("Connect your application to %s:%d", privateIP, port),
			},
		)
	case MethodConnector:
		steps = append(steps,
			SetupStep{
				Order:       3,
				Title:       "Install connector library",
				Description: "Add the Cloud SQL connector library to your application",
			},
			SetupStep{
				Order:       4,
				Title:       "Configure connection",
				Description: fmt.Sprintf("Use instance connection name: %s", connectionName),
			},
		)
	}

	return steps
}

// GenerateGKESetupSteps generates setup steps for GKE connection.
// The namespace parameter specifies the Kubernetes namespace for deployment.
func GenerateGKESetupSteps(connectionName string, port int, projectID string, namespace string) []SetupStep {
	// Use "default" namespace if not specified
	if namespace == "" {
		namespace = "default"
	}

	return []SetupStep{
		{
			Order:       1,
			Title:       "Enable required APIs",
			Description: "Enable Cloud SQL Admin API and IAM Credentials API",
			Command:     "gcloud services enable sqladmin.googleapis.com iamcredentials.googleapis.com",
		},
		{
			Order:       2,
			Title:       "Enable Workload Identity on cluster",
			Description: "If not already enabled, update cluster to enable Workload Identity",
			Command:     fmt.Sprintf("gcloud container clusters update CLUSTER_NAME --workload-pool=%s.svc.id.goog", projectID),
		},
		{
			Order:       3,
			Title:       "Create GCP Service Account",
			Description: "Create a service account for Cloud SQL access",
			Command:     "gcloud iam service-accounts create cloudsql-sa --display-name='Cloud SQL Service Account'",
		},
		{
			Order:       4,
			Title:       "Grant Cloud SQL Client role",
			Description: "Grant the service account permission to connect to Cloud SQL",
			Command:     fmt.Sprintf("gcloud projects add-iam-policy-binding %s --member='serviceAccount:cloudsql-sa@%s.iam.gserviceaccount.com' --role='roles/cloudsql.client'", projectID, projectID),
		},
		{
			Order:       5,
			Title:       "Create Kubernetes Service Account",
			Description: fmt.Sprintf("Create a Kubernetes service account in the '%s' namespace", namespace),
			Command:     fmt.Sprintf("kubectl create serviceaccount ksa-cloudsql -n %s", namespace),
		},
		{
			Order:       6,
			Title:       "Bind Kubernetes SA to GCP SA",
			Description: "Allow the Kubernetes service account to impersonate the GCP service account",
			Command:     fmt.Sprintf("gcloud iam service-accounts add-iam-policy-binding cloudsql-sa@%s.iam.gserviceaccount.com --role='roles/iam.workloadIdentityUser' --member='serviceAccount:%s.svc.id.goog[%s/ksa-cloudsql]'", projectID, projectID, namespace),
		},
		{
			Order:       7,
			Title:       "Annotate Kubernetes SA",
			Description: "Add annotation to link Kubernetes SA to GCP SA",
			Command:     fmt.Sprintf("kubectl annotate serviceaccount ksa-cloudsql -n %s iam.gke.io/gcp-service-account=cloudsql-sa@%s.iam.gserviceaccount.com", namespace, projectID),
		},
		{
			Order:       8,
			Title:       "Deploy with Auth Proxy sidecar",
			Description: "Add Cloud SQL Auth Proxy sidecar to your deployment (see sidecarYaml in config)",
		},
	}
}

// GenerateCloudRunSetupSteps generates setup steps for Cloud Run connection.
func GenerateCloudRunSetupSteps(connectionName, serviceAccount, region string) []SetupStep {
	return []SetupStep{
		{
			Order:       1,
			Title:       "Enable Cloud SQL Admin API",
			Description: "Ensure the Cloud SQL Admin API is enabled",
			Command:     "gcloud services enable sqladmin.googleapis.com",
		},
		{
			Order:       2,
			Title:       "Grant IAM permissions",
			Description: fmt.Sprintf("Grant Cloud SQL Client role to Cloud Run service account: %s", serviceAccount),
			Command:     fmt.Sprintf("gcloud projects add-iam-policy-binding PROJECT_ID --member='serviceAccount:%s' --role='roles/cloudsql.client'", serviceAccount),
		},
		{
			Order:       3,
			Title:       "Deploy with Cloud SQL connection",
			Description: "Deploy your service with the Cloud SQL instance connected",
			Command:     fmt.Sprintf("gcloud run deploy SERVICE_NAME --image=IMAGE_URL --add-cloudsql-instances=%s --region=%s", connectionName, region),
		},
		{
			Order:       4,
			Title:       "Configure connection in code",
			Description: fmt.Sprintf("Connect via Unix socket: /cloudsql/%s", connectionName),
		},
	}
}

// GenerateLocalSetupSteps generates setup steps for local development.
// This focuses on public IP connections via Auth Proxy, which is the recommended
// approach for local development. Private IP connections require VPN or Cloud
// Interconnect setup, which is outside the scope of this tool.
func GenerateLocalSetupSteps(connectionName string, port int) []SetupStep {
	return []SetupStep{
		{
			Order:       1,
			Title:       "Enable Cloud SQL Admin API",
			Description: "Ensure the Cloud SQL Admin API is enabled in your project",
			Command:     "gcloud services enable sqladmin.googleapis.com",
		},
		{
			Order:       2,
			Title:       "Authenticate with Google Cloud",
			Description: "Set up Application Default Credentials",
			Command:     "gcloud auth application-default login",
		},
		{
			Order:       3,
			Title:       "Install Cloud SQL Auth Proxy",
			Description: "Download the Cloud SQL Auth Proxy for your platform",
			Command:     "# macOS: curl -o cloud-sql-proxy https://storage.googleapis.com/cloud-sql-connectors/cloud-sql-proxy/v2.8.0/cloud-sql-proxy.darwin.amd64\n# Linux: curl -o cloud-sql-proxy https://storage.googleapis.com/cloud-sql-connectors/cloud-sql-proxy/v2.8.0/cloud-sql-proxy.linux.amd64\nchmod +x cloud-sql-proxy",
		},
		{
			Order:       4,
			Title:       "Start Auth Proxy",
			Description: "Run the Auth Proxy in a terminal (connects via public IP)",
			Command:     fmt.Sprintf("./cloud-sql-proxy %s", connectionName),
		},
		{
			Order:       5,
			Title:       "Set environment variables",
			Description: "Configure database credentials",
			Command:     "export DB_USER='your-db-user'\nexport DB_PASS='your-db-password'\nexport DB_NAME='your-db-name'",
		},
		{
			Order:       6,
			Title:       "Connect your application",
			Description: fmt.Sprintf("Connect to localhost:%d with your database credentials", port),
		},
	}
}

// GenerateGKESidecarYAML generates the sidecar container YAML for GKE deployments.
func GenerateGKESidecarYAML(connectionName string, port int) string {
	return fmt.Sprintf(`- name: cloud-sql-proxy
  image: gcr.io/cloud-sql-connectors/cloud-sql-proxy:2.8.0
  args:
    - "--structured-logs"
    - "--auto-iam-authn"
    - "--port=%d"
    - "%s"
  securityContext:
    runAsNonRoot: true
  resources:
    requests:
      memory: "256Mi"
      cpu: "100m"
    limits:
      memory: "512Mi"
      cpu: "500m"`, port, connectionName)
}

// GenerateKubernetesSecretYAML generates a Kubernetes Secret manifest.
func GenerateKubernetesSecretYAML(dbName string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: cloudsql-db-credentials
type: Opaque
stringData:
  DB_USER: "<your-database-user>"
  DB_PASS: "<your-database-password>"
  DB_NAME: "%s"`, dbName)
}

// GenerateEnvironmentConfig generates environment configuration for the connection.
func GenerateEnvironmentConfig(method ConnectionMethod, computeType ComputeType, connectionName string, port int, privateIP, dbName, projectID string) EnvironmentConfig {
	config := EnvironmentConfig{
		EnvironmentVariables: map[string]string{
			"DB_USER": "<your-database-user>",
			"DB_PASS": "<your-database-password>",
			"DB_NAME": dbName,
		},
	}

	switch method {
	case MethodAuthProxy:
		config.EnvironmentVariables["DB_HOST"] = "127.0.0.1"
		config.EnvironmentVariables["DB_PORT"] = fmt.Sprintf("%d", port)
		if privateIP != "" {
			config.AuthProxyCommand = fmt.Sprintf("./cloud-sql-proxy --private-ip %s", connectionName)
		} else {
			config.AuthProxyCommand = fmt.Sprintf("./cloud-sql-proxy %s", connectionName)
		}

	case MethodDirectPrivateIP:
		config.EnvironmentVariables["DB_HOST"] = privateIP
		config.EnvironmentVariables["DB_PORT"] = fmt.Sprintf("%d", port)

	case MethodUnixSocket:
		config.EnvironmentVariables["INSTANCE_UNIX_SOCKET"] = fmt.Sprintf("/cloudsql/%s", connectionName)

	case MethodConnector:
		config.EnvironmentVariables["INSTANCE_CONNECTION_NAME"] = connectionName
	}

	// Add compute-specific config
	switch computeType {
	case ComputeGKE:
		config.SidecarYAML = GenerateGKESidecarYAML(connectionName, port)
		config.SecretYAML = GenerateKubernetesSecretYAML(dbName)
		config.KubernetesServiceAccount = fmt.Sprintf("iam.gke.io/gcp-service-account: cloudsql-sa@%s.iam.gserviceaccount.com", projectID)

	case ComputeCloudRun:
		config.CloudRunFlags = []string{
			fmt.Sprintf("--add-cloudsql-instances=%s", connectionName),
		}
	}

	return config
}
