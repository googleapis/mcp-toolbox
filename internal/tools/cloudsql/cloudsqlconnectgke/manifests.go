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
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/util/cloudsqlconnect"
)

// authProxyVersion is the Cloud SQL Auth Proxy release every generated
// install command and container image is pinned to. Keep it in one place
// so bumping the proxy is a one-line change.
const authProxyVersion = "2.26.0"

// authProxyImage is the container image for the Auth Proxy sidecar.
const authProxyImage = "gcr.io/cloud-sql-connectors/cloud-sql-proxy:" + authProxyVersion

// Defaults for the Kubernetes and IAM identities the GKE setup creates.
const (
	defaultNamespace = "default"
	defaultKSA       = "cloudsql-ksa"
	defaultGSAID     = "cloudsql-client"
	// secretName is the Kubernetes Secret holding DB credentials.
	secretName = "cloudsql-db-credentials"
	// proxyHealthPort serves the proxy's /startup, /liveness and
	// /readiness endpoints.
	proxyHealthPort = 9090
)

// networkPath is how traffic from GKE pods reaches the Cloud SQL
// instance. It decides the Auth Proxy IP-type flag and the setup steps.
type networkPath string

const (
	// pathPrivateIP uses the instance's private services access IP. Needs
	// the same VPC (Shared VPC counts) and a VPC-native cluster.
	pathPrivateIP networkPath = "private_ip"
	// pathPublicIP uses the instance's public IP via the proxy's mTLS
	// tunnel. Needs egress to the internet from the nodes.
	pathPublicIP networkPath = "public_ip"
	// pathPSC uses a Private Service Connect endpoint in the cluster VPC.
	pathPSC networkPath = "psc"
	// pathNone means no usable path exists without infrastructure changes.
	pathNone networkPath = "none"
)

// resolveNetworkPath picks the network path, in order of preference:
//
//  1. Private IP — lowest latency, never leaves the VPC. Only viable if
//     the cluster shares the instance's VPC and is VPC-native: pod IPs in
//     a routes-based cluster are not exported over the private services
//     access peering, so pods time out even though nodes can connect.
//  2. Public IP — works from any VPC; the proxy still encrypts with
//     mTLS and authorizes via IAM, so no authorized networks are needed.
//  3. PSC — works across VPCs/projects without public exposure, but needs
//     an endpoint and DNS record in the cluster VPC.
func resolveNetworkPath(sqlInfo *instanceInfo, gkeInfo *clusterInfo) networkPath {
	if privateIPReachable(sqlInfo, gkeInfo) {
		return pathPrivateIP
	}
	if sqlInfo.PublicIPEnabled {
		return pathPublicIP
	}
	if sqlInfo.PSCEnabled {
		return pathPSC
	}
	return pathNone
}

// privateIPReachable reports whether pods can reach the instance's private IP.
func privateIPReachable(sqlInfo *instanceInfo, gkeInfo *clusterInfo) bool {
	return sqlInfo.PrivateIPEnabled && gkeInfo.VPCNative && sameNetwork(sqlInfo.VPCNetwork, gkeNetwork(gkeInfo))
}

// gkeNetwork prefers the fully-qualified network path (Shared VPC safe).
func gkeNetwork(gkeInfo *clusterInfo) string {
	if gkeInfo.NetworkURI != "" {
		return gkeInfo.NetworkURI
	}
	return gkeInfo.VPCNetwork
}

// proxyIPFlag returns the Auth Proxy flag selecting the IP type. Public
// is the proxy default, so it needs no flag.
func proxyIPFlag(path networkPath) string {
	switch path {
	case pathPrivateIP:
		return "--private-ip"
	case pathPSC:
		return "--psc"
	default:
		return ""
	}
}

// gkeIdentity names the identities the setup creates and wires together.
type gkeIdentity struct {
	ClusterProject string
	Namespace      string
	KSA            string
	// GSAEmail is the Google service account the KSA impersonates.
	GSAEmail string
}

// newGKEIdentity fills defaults and derives the GSA email from the
// cluster project (Workload Identity binds a KSA to a GSA via the
// cluster project's workload pool).
func newGKEIdentity(clusterProject, namespace, ksa string) gkeIdentity {
	if namespace == "" {
		namespace = defaultNamespace
	}
	if ksa == "" {
		ksa = defaultKSA
	}
	return gkeIdentity{
		ClusterProject: clusterProject,
		Namespace:      namespace,
		KSA:            ksa,
		GSAEmail:       fmt.Sprintf("%s@%s.iam.gserviceaccount.com", defaultGSAID, clusterProject),
	}
}

// WorkloadIdentityMember is the IAM member string for the KSA.
func (id gkeIdentity) WorkloadIdentityMember() string {
	return fmt.Sprintf("serviceAccount:%s.svc.id.goog[%s/%s]", id.ClusterProject, id.Namespace, id.KSA)
}

// manifestOptions drives Kubernetes manifest generation.
type manifestOptions struct {
	Method         cloudsqlconnect.ConnectionMethod
	Path           networkPath
	ConnectionName string
	Port           int
	PrivateIP      string
	DBName         string
	Identity       gkeIdentity
	NativeSidecar  bool
}

// serviceAccountYAML returns the annotated KSA manifest.
func serviceAccountYAML(id gkeIdentity) string {
	return fmt.Sprintf(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: %s
  namespace: %s
  annotations:
    iam.gke.io/gcp-service-account: %s`, id.KSA, id.Namespace, id.GSAEmail)
}

// secretYAML returns a Secret manifest with credential
// placeholders. Prefer `kubectl create secret` (see setup steps) so real
// passwords never land in a file that might be committed.
func secretYAML(namespace, dbName string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  DB_USER: "<your-database-user>"
  DB_PASS: "<your-database-password>"
  DB_NAME: "%s"`, secretName, namespace, dbName)
}

// sidecarYAML returns the Auth Proxy container spec, indented
// to sit under `containers:` (classic) or `initContainers:` (native).
//
// Choices that avoid the most common GKE failures:
//   - IP-type flag matches the resolved path; without --private-ip a
//     private-only instance fails with "instance does not have IP of type
//     PUBLIC".
//   - Health checks on 0.0.0.0 back a startupProbe, so with native
//     sidecars kubelet holds the app until the proxy is ready.
//   - --exit-zero-on-sigterm lets Jobs and rolling updates finish cleanly.
//   - No --auto-iam-authn: it replaces the app's password with an IAM
//     token and breaks built-in database users. It's offered as an
//     opt-in consideration when the instance has IAM auth enabled.
func sidecarYAML(opts manifestOptions) string {
	args := []string{"--structured-logs", fmt.Sprintf("--port=%d", opts.Port)}
	if flag := proxyIPFlag(opts.Path); flag != "" {
		args = append(args, flag)
	}
	args = append(args,
		"--health-check",
		"--http-address=0.0.0.0",
		fmt.Sprintf("--http-port=%d", proxyHealthPort),
		"--exit-zero-on-sigterm",
		opts.ConnectionName,
	)
	var argLines strings.Builder
	for _, a := range args {
		fmt.Fprintf(&argLines, "\n    - %q", a)
	}

	restartPolicy := ""
	if opts.NativeSidecar {
		// restartPolicy: Always on an init container is what makes it a
		// native sidecar.
		restartPolicy = "\n  restartPolicy: Always"
	}

	return fmt.Sprintf(`- name: cloud-sql-proxy
  image: %s%s
  args:%s
  securityContext:
    runAsNonRoot: true
    allowPrivilegeEscalation: false
  startupProbe:
    httpGet:
      path: /startup
      port: %d
    periodSeconds: 1
    failureThreshold: 60
  livenessProbe:
    httpGet:
      path: /liveness
      port: %d
    periodSeconds: 10
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      memory: "256Mi"`, authProxyImage, restartPolicy, argLines.String(), proxyHealthPort, proxyHealthPort)
}

// deploymentYAML returns a complete example Deployment that
// wires the KSA, the credentials Secret, and (for Auth Proxy) the
// sidecar together, so the user can diff it against their own manifest
// instead of merging fragments by hand. App-specific values are
// UPPERCASE placeholders.
func deploymentYAML(opts manifestOptions) string {
	env := deploymentEnv(opts)

	var initContainers, sidecar string
	if opts.Method == cloudsqlconnect.MethodAuthProxy {
		block := indent(sidecarYAML(opts), 6)
		if opts.NativeSidecar {
			initContainers = "\n      initContainers:\n" + block
		} else {
			sidecar = "\n" + block
		}
	}

	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      serviceAccountName: %s%s
      containers:
      - name: app
        image: IMAGE_URL
        envFrom:
        - secretRef:
            name: %s
        env:%s%s`, opts.Identity.Namespace, opts.Identity.KSA, initContainers, secretName, env, sidecar)
}

func deploymentEnv(opts manifestOptions) string {
	var vars [][2]string
	switch opts.Method {
	case cloudsqlconnect.MethodAuthProxy:
		vars = [][2]string{{"DB_HOST", "127.0.0.1"}, {"DB_PORT", fmt.Sprintf("%d", opts.Port)}}
	case cloudsqlconnect.MethodDirectPrivateIP:
		vars = [][2]string{{"DB_HOST", opts.PrivateIP}, {"DB_PORT", fmt.Sprintf("%d", opts.Port)}}
	case cloudsqlconnect.MethodConnector:
		vars = [][2]string{{"INSTANCE_CONNECTION_NAME", opts.ConnectionName}}
	}
	var b strings.Builder
	for _, v := range vars {
		fmt.Fprintf(&b, "\n        - name: %s\n          value: %q", v[0], v[1])
	}
	return b.String()
}

// indent prefixes every line of s with n spaces.
func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

// generateGKEEnvironmentConfig builds the environment config for a GKE
// workload: env vars plus every manifest the setup steps reference. The
// env var names match the shared generator (DB_USER, DB_PASS, DB_NAME,
// DB_HOST, DB_PORT, INSTANCE_CONNECTION_NAME) so code snippets line up.
// No host-level proxy command is emitted; on GKE the sidecar carries the
// proxy invocation.
func generateGKEEnvironmentConfig(opts manifestOptions) environmentConfig {
	env := map[string]string{
		"DB_USER": "<your-database-user>",
		"DB_PASS": "<your-database-password>",
		"DB_NAME": opts.DBName,
	}
	switch opts.Method {
	case cloudsqlconnect.MethodAuthProxy:
		env["DB_HOST"] = "127.0.0.1"
		env["DB_PORT"] = fmt.Sprintf("%d", opts.Port)
	case cloudsqlconnect.MethodDirectPrivateIP:
		env["DB_HOST"] = opts.PrivateIP
		env["DB_PORT"] = fmt.Sprintf("%d", opts.Port)
	case cloudsqlconnect.MethodConnector:
		env["INSTANCE_CONNECTION_NAME"] = opts.ConnectionName
	}

	cfg := environmentConfig{
		EnvironmentConfig: cloudsqlconnect.EnvironmentConfig{
			EnvironmentVariables:     env,
			KubernetesServiceAccount: fmt.Sprintf("iam.gke.io/gcp-service-account: %s", opts.Identity.GSAEmail),
			SecretYAML:               secretYAML(opts.Identity.Namespace, opts.DBName),
		},
		ServiceAccountYAML: serviceAccountYAML(opts.Identity),
		DeploymentYAML:     deploymentYAML(opts),
	}
	if opts.Method == cloudsqlconnect.MethodAuthProxy {
		cfg.SidecarYAML = sidecarYAML(opts)
	}
	return cfg
}
