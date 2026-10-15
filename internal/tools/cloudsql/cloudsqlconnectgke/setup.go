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
	"slices"
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/util/cloudsqlconnect"
)

// setupParams carries everything the GKE setup generator needs.
type setupParams struct {
	SQL      *instanceInfo
	Cluster  *clusterInfo
	Identity gkeIdentity
	Method   cloudsqlconnect.ConnectionMethod
	Path     networkPath
	// Manifests are the generated manifests the steps embed or reference.
	Manifests environmentConfig
	// NativeSidecar reports whether the sidecar is a native sidecar.
	NativeSidecar bool
}

// stepList auto-numbers steps so conditional steps never leave gaps.
type stepList []cloudsqlconnect.SetupStep

func (s *stepList) add(title, description, command string) {
	*s = append(*s, cloudsqlconnect.SetupStep{Order: len(*s) + 1, Title: title, Description: description, Command: command})
}

// generateGKESetupSteps returns the ordered steps to connect a GKE
// workload to Cloud SQL. Commands avoid pipes, heredocs and command
// substitution so an agent can show and run each line verbatim. Steps are tailored to the detected state: work
// that is already done (Workload Identity on, node pools already on
// GKE_METADATA, default namespace) is skipped, and steps that only apply
// to a particular network path (PSC endpoint, Cloud NAT) appear only
// when needed. Every command carries explicit --project / --location /
// -n flags so it is safe to run regardless of the user's gcloud and
// kubectl defaults, which is where cross-project setups usually go wrong.
func generateGKESetupSteps(p setupParams) []cloudsqlconnect.SetupStep {
	sql, cl, id := p.SQL, p.Cluster, p.Identity
	clusterFlags := fmt.Sprintf("--location=%s --project=%s", cl.Location, cl.Project)
	var steps stepList

	// APIs. The proxy's Admin API calls are attributed to the service
	// account's project, so sqladmin must be on in the cluster project
	// too when it differs from the instance project.
	apisCmd := fmt.Sprintf("gcloud services enable container.googleapis.com sqladmin.googleapis.com iamcredentials.googleapis.com --project=%s", cl.Project)
	if sql.Project != cl.Project {
		apisCmd += fmt.Sprintf("\ngcloud services enable sqladmin.googleapis.com --project=%s", sql.Project)
	}
	steps.add("Enable required APIs",
		"Enable the Kubernetes Engine, Cloud SQL Admin and IAM Service Account Credentials APIs (the last one is what Workload Identity uses to mint tokens).",
		apisCmd)

	if p.Path == pathNone {
		steps.add("Resolve the connectivity blocker first",
			"No network path from this cluster to the instance exists today, so the workload will not connect until one of these is done. Pick ONE: (a) enable public IP on the instance (the Auth Proxy still encrypts and IAM-authorizes traffic, no authorized networks needed); (b) give the instance a private IP in the cluster's VPC (requires private services access on that VPC); or (c) enable Private Service Connect. Re-run this tool afterwards to get updated steps.",
			fmt.Sprintf("# (a) public IP\ngcloud sql instances patch %s --project=%s --assign-ip\n# (b) private IP in the cluster VPC\ngcloud sql instances patch %s --project=%s --network=%s",
				sql.Name, sql.Project, sql.Name, sql.Project, clusterNetworkPath(cl)))
	}

	steps.add("Point kubectl at the cluster",
		"Fetch credentials so every kubectl command below targets this cluster.",
		fmt.Sprintf("gcloud container clusters get-credentials %s %s", cl.Name, clusterFlags))

	if !cl.WorkloadIdentity && !cl.Autopilot {
		steps.add("Enable Workload Identity Federation on the cluster",
			"Workload Identity lets pods authenticate as a Google service account without exported keys. This updates the control plane and can take several minutes.",
			fmt.Sprintf("gcloud container clusters update %s %s --workload-pool=%s.svc.id.goog", cl.Name, clusterFlags, cl.Project))
	}

	if len(cl.NodePoolsWithoutGKEMetadata) > 0 {
		cmds := make([]string, 0, len(cl.NodePoolsWithoutGKEMetadata))
		for _, np := range cl.NodePoolsWithoutGKEMetadata {
			cmds = append(cmds, fmt.Sprintf("gcloud container node-pools update %s --cluster=%s %s --workload-metadata=GKE_METADATA", np, cl.Name, clusterFlags))
		}
		steps.add("Switch node pools to the GKE metadata server",
			fmt.Sprintf("Pods on node pools %v would otherwise silently use the node's service account instead of Workload Identity. This recreates the nodes in each pool (respecting surge settings); schedule it for a low-traffic window.", cl.NodePoolsWithoutGKEMetadata),
			strings.Join(cmds, "\n"))
	}

	steps.add("Create a Google service account for the workload",
		fmt.Sprintf("Skip if you already have one; then substitute its email for %s in the remaining steps.", id.GSAEmail),
		fmt.Sprintf("gcloud iam service-accounts create %s --project=%s --display-name=\"Cloud SQL client for GKE\"", defaultGSAID, cl.Project))

	steps.add("Grant the service account access to Cloud SQL",
		fmt.Sprintf("roles/cloudsql.client must be granted in the instance's project (%s), not the cluster's.", sql.Project),
		fmt.Sprintf("gcloud projects add-iam-policy-binding %s --member=\"serviceAccount:%s\" --role=\"roles/cloudsql.client\"", sql.Project, id.GSAEmail))

	if id.Namespace != defaultNamespace {
		steps.add("Create the namespace",
			"Skip if the namespace already exists.",
			fmt.Sprintf("kubectl create namespace %s", id.Namespace))
	}

	steps.add("Create the annotated Kubernetes service account",
		"The iam.gke.io/gcp-service-account annotation tells the GKE metadata server which Google service account this KSA acts as. Skip the create if the KSA exists; the annotate is safe to re-run. environmentConfig.serviceAccountYaml is the equivalent manifest.",
		fmt.Sprintf("kubectl create serviceaccount %[1]s -n %[2]s\nkubectl annotate serviceaccount %[1]s -n %[2]s iam.gke.io/gcp-service-account=%[3]s --overwrite", id.KSA, id.Namespace, id.GSAEmail))

	steps.add("Allow the Kubernetes service account to impersonate the Google service account",
		"Both halves are required: this IAM binding and the annotation above. The namespace and KSA name in the member string must match exactly.",
		fmt.Sprintf("gcloud iam service-accounts add-iam-policy-binding %s --project=%s --role=\"roles/iam.workloadIdentityUser\" --member=\"%s\"", id.GSAEmail, cl.Project, id.WorkloadIdentityMember()))

	steps.add("Store database credentials in a Kubernetes Secret",
		"Replace the placeholders with a real database user and password. Creating the Secret from the command line keeps the password out of files that could be committed.",
		fmt.Sprintf("kubectl create secret generic %s -n %s --from-literal=DB_USER='<your-database-user>' --from-literal=DB_PASS='<your-database-password>' --from-literal=DB_NAME='%s'", secretName, id.Namespace, p.Manifests.EnvironmentVariables["DB_NAME"]))

	switch p.Path {
	case pathPSC:
		addPSCSteps(&steps, sql, cl)
	case pathPublicIP:
		if cl.PrivateCluster {
			region := regionOf(cl.Location)
			netProject := networkProject(cl)
			steps.add("Give private nodes internet egress (skip if Cloud NAT already exists)",
				"Private nodes have no external IPs, so the proxy cannot reach the instance's public IP without Cloud NAT on the cluster's network and region.",
				fmt.Sprintf("gcloud compute routers create cloudsql-nat-router --project=%s --network=%s --region=%s\ngcloud compute routers nats create cloudsql-nat --project=%s --router=cloudsql-nat-router --region=%s --auto-allocate-nat-external-ips --nat-all-subnet-ip-ranges",
					netProject, cl.VPCNetwork, region, netProject, region))
		}
	}

	addDeploySteps(&steps, p)
	return steps
}

func addPSCSteps(steps *stepList, sql *instanceInfo, cl *clusterInfo) {
	if !slices.Contains(sql.PSCAllowedConsumers, cl.Project) {
		allowed := append(slices.Clone(sql.PSCAllowedConsumers), cl.Project)
		steps.add("Allow the cluster's project to connect over PSC",
			"The flag replaces the whole list, so the command keeps the currently allowed projects and appends the cluster's project.",
			fmt.Sprintf("gcloud sql instances patch %s --project=%s --allowed-psc-projects=%s", sql.Name, sql.Project, strings.Join(allowed, ",")))
	}

	region := sql.Region
	netProject := networkProject(cl)
	subnet := cl.Subnetwork
	if regionOf(cl.Location) != region || subnet == "" {
		// The endpoint must live in the instance's region.
		subnet = fmt.Sprintf("SUBNET_IN_%s", strings.ToUpper(strings.ReplaceAll(region, "-", "_")))
	}
	attachment := sql.PSCServiceAttachment
	if attachment == "" {
		attachment = "SERVICE_ATTACHMENT_URI"
	}
	dnsName := sql.DNSName
	if dnsName == "" {
		dnsName = "INSTANCE_DNS_NAME"
	}
	steps.add("Create a PSC endpoint in the cluster's VPC",
		fmt.Sprintf("Reserves an internal IP in %s and points it at the instance's service attachment. --allow-psc-global-access lets nodes in other regions use it. Skip if an endpoint already exists.", region),
		fmt.Sprintf("gcloud compute addresses create cloudsql-psc-ip --project=%[1]s --region=%[2]s --subnet=%[3]s\ngcloud compute forwarding-rules create cloudsql-psc-endpoint --project=%[1]s --region=%[2]s --network=%[4]s --address=cloudsql-psc-ip --target-service-attachment=%[5]s --allow-psc-global-access",
			netProject, region, subnet, cl.VPCNetwork, attachment))
	steps.add("Resolve the instance's DNS name to the endpoint",
		"The Auth Proxy's --psc mode dials the instance's DNS name, so it must resolve to the endpoint IP inside the cluster's VPC. The first command prints the endpoint IP; substitute it for PSC_ENDPOINT_IP.",
		fmt.Sprintf("gcloud compute addresses describe cloudsql-psc-ip --project=%[1]s --region=%[4]s --format=\"value(address)\"\ngcloud dns managed-zones create cloudsql-psc --project=%[1]s --dns-name=%[2]s --networks=%[3]s --visibility=private --description=\"Cloud SQL PSC\"\ngcloud dns record-sets create %[2]s --project=%[1]s --zone=cloudsql-psc --type=A --ttl=300 --rrdatas=PSC_ENDPOINT_IP",
			netProject, dnsName, cl.VPCNetwork, region))
}

func addDeploySteps(steps *stepList, p setupParams) {
	ns := p.Identity.Namespace
	switch p.Method {
	case cloudsqlconnect.MethodAuthProxy:
		placement := "under spec.template.spec.containers, next to your app container"
		if p.NativeSidecar {
			placement = "under spec.template.spec.initContainers (restartPolicy: Always makes it a native sidecar that starts before your app and exits after it)"
		}
		steps.add("Add the Auth Proxy sidecar to your workload",
			fmt.Sprintf("Start from environmentConfig.deploymentYaml, or merge environmentConfig.sidecarYaml into your existing pod spec %s. Set serviceAccountName to %s and point the app at 127.0.0.1:%s.", placement, p.Identity.KSA, p.Manifests.EnvironmentVariables["DB_PORT"]),
			fmt.Sprintf("kubectl apply -n %s -f deployment.yaml", ns))
	case cloudsqlconnect.MethodConnector:
		steps.add("Deploy with the connector library",
			fmt.Sprintf("Add the Cloud SQL connector to your app (see codeSnippet), set serviceAccountName to %s, and deploy. No sidecar is needed.", p.Identity.KSA),
			fmt.Sprintf("kubectl apply -n %s -f deployment.yaml", ns))
	case cloudsqlconnect.MethodDirectPrivateIP:
		steps.add("Deploy pointing at the private IP",
			fmt.Sprintf("Set DB_HOST to %s. Workload Identity is not used on this path; the app authenticates with the database user and password only.", p.SQL.PrivateIPAddress),
			fmt.Sprintf("kubectl apply -n %s -f deployment.yaml", ns))
	}

	verify := fmt.Sprintf("kubectl rollout status deployment/my-app -n %s", ns)
	if p.Method == cloudsqlconnect.MethodAuthProxy {
		verify += fmt.Sprintf("\nkubectl logs deployment/my-app -n %s -c cloud-sql-proxy --tail=50", ns)
	}
	steps.add("Verify the connection",
		"Wait for the rollout, then check the proxy logs for \"ready for new connections\". If something fails, match the error against the troubleshooting list.",
		verify)
}

// clusterNetworkPath returns a network path usable by gcloud --network.
func clusterNetworkPath(cl *clusterInfo) string {
	if cl.NetworkURI != "" {
		return cl.NetworkURI
	}
	return fmt.Sprintf("projects/%s/global/networks/%s", cl.Project, cl.VPCNetwork)
}

// networkProject is the project that owns the cluster's VPC — the host
// project for Shared VPC — where NAT, PSC endpoints and DNS must be created.
func networkProject(cl *clusterInfo) string {
	if p, _ := splitNetworkPath(cl.NetworkURI); p != "" {
		return p
	}
	return cl.Project
}

// regionOf maps a zone ("us-central1-a") or region ("us-central1") to
// its region.
func regionOf(location string) string {
	parts := strings.Split(location, "-")
	if len(parts) == 3 && len(parts[2]) == 1 {
		return parts[0] + "-" + parts[1]
	}
	return location
}

// gkeTroubleshooting returns the failure modes users hit most often
// after wiring GKE to Cloud SQL, filtered to the chosen method.
func gkeTroubleshooting(p setupParams) []troubleshootingTip {
	id := p.Identity
	tips := []troubleshootingTip{
		{
			Symptom: "Proxy logs \"Permission 'iam.serviceAccounts.getAccessToken' denied\" or \"Unable to generate access token\"",
			Cause:   "Workload Identity is not wired end to end: the roles/iam.workloadIdentityUser binding is missing, the KSA annotation names a different Google service account, or the pod runs in a different namespace or KSA than the binding.",
			Fix:     fmt.Sprintf("Confirm the pod uses serviceAccountName: %s in namespace %s, the KSA annotation is %s, and re-run the impersonation binding step.", id.KSA, id.Namespace, id.GSAEmail),
		},
		{
			Symptom: "\"NOT_AUTHORIZED\" / \"403\" calling cloudsql.instances.get or connect",
			Cause:   fmt.Sprintf("The Google service account lacks roles/cloudsql.client in project %s, or the pod is falling back to the node's service account.", p.SQL.Project),
			Fix:     "Re-run the cloudsql.client grant step. If errors mention a *-compute@developer.gserviceaccount.com account, the node pool is not on GKE_METADATA or serviceAccountName is not set.",
		},
		{
			Symptom: "\"dial tcp ...:3307: i/o timeout\" from the proxy",
			Cause:   "The network path is not reachable from the pods: private IP from a different VPC or a routes-based cluster, public IP from private nodes without Cloud NAT, or a PSC endpoint without DNS.",
			Fix:     "Re-run this tool and follow the network-path steps; check egress firewall rules allow TCP 3307 to the instance.",
		},
		{
			Symptom: "\"instance does not have IP of type PUBLIC\" (or PRIVATE / PSC)",
			Cause:   "The proxy's IP-type flag does not match how the instance is exposed.",
			Fix:     "Use the args from environmentConfig.sidecarYaml exactly; they already match the detected configuration.",
		},
		{
			Symptom: "\"password authentication failed\" or \"Access denied for user\"",
			Cause:   "Wrong database credentials in the Secret, or --auto-iam-authn added to the proxy while using a built-in database user.",
			Fix:     fmt.Sprintf("Recreate the %s Secret with the correct user and password and restart the pods. Only add --auto-iam-authn if the database user is an IAM user.", secretName),
		},
	}
	if p.Method == cloudsqlconnect.MethodAuthProxy {
		if p.NativeSidecar {
			tips = append(tips, troubleshootingTip{
				Symptom: "Pod stuck in Init with the cloud-sql-proxy container not ready",
				Cause:   "The proxy's startup probe never passed, so kubelet is holding the app container back by design.",
				Fix:     "Read the proxy logs; the root cause is almost always one of the identity or network errors above.",
			})
		} else {
			tips = append(tips,
				troubleshootingTip{
					Symptom: "App logs \"connection refused\" on 127.0.0.1 right after the pod starts",
					Cause:   "Classic sidecars start in parallel, so the app can dial before the proxy is listening.",
					Fix:     "Retry database connections with backoff at startup, or upgrade the cluster to 1.29+ to use a native sidecar.",
				},
				troubleshootingTip{
					Symptom: "A Job or CronJob pod never completes",
					Cause:   "The classic proxy sidecar keeps running after the main container exits.",
					Fix:     "Upgrade to GKE 1.29+ and move the proxy to initContainers with restartPolicy: Always, or add --quitquitquit and have the job POST to localhost:9091/quitquitquit when done.",
				})
		}
	}
	return tips
}

// summarizeGKE returns the plain-language verdict an agent should lead
// with before walking the user through the steps.
func summarizeGKE(p setupParams, validation *cloudsqlconnect.ValidationResult, recommendation cloudsqlconnect.ConnectionRecommendation, stepCount int) string {
	cl := p.Cluster
	if !validation.Valid || p.Path == pathNone {
		return fmt.Sprintf("Blocked: cluster %s cannot reach instance %s yet. %s Resolve that first (see the first setup steps), then re-run this tool.",
			cl.Name, p.SQL.Name, strings.Join(validation.Issues, " "))
	}

	var pending []string
	if !cl.WorkloadIdentity && !cl.Autopilot {
		pending = append(pending, "enable Workload Identity")
	}
	if len(cl.NodePoolsWithoutGKEMetadata) > 0 {
		pending = append(pending, fmt.Sprintf("move %d node pool(s) to GKE_METADATA", len(cl.NodePoolsWithoutGKEMetadata)))
	}
	switch p.Path {
	case pathPSC:
		pending = append(pending, "create a PSC endpoint and DNS record")
	case pathPublicIP:
		if cl.PrivateCluster {
			pending = append(pending, "confirm Cloud NAT for private nodes")
		}
	}

	verdict := fmt.Sprintf("Ready to configure: use %s over %s.", recommendation.Name, pathLabel(p.Path))
	if len(pending) > 0 {
		verdict += " Cluster changes needed first: " + strings.Join(pending, "; ") + "."
	}
	return verdict + fmt.Sprintf(" Follow the %d setup steps in order; each command is idempotent or says when to skip it. Ask the user before running commands that modify the cluster, IAM, or the instance.", stepCount)
}

func pathLabel(path networkPath) string {
	switch path {
	case pathPrivateIP:
		return "the instance's private IP"
	case pathPublicIP:
		return "the instance's public IP (encrypted and IAM-authorized by the proxy)"
	case pathPSC:
		return "Private Service Connect"
	default:
		return "no available network path"
	}
}
