# MCP Toolbox Helm chart

Deploys [MCP Toolbox for Databases](https://github.com/googleapis/mcp-toolbox)
to Kubernetes.

## Install

Write your tool configuration to `tools.yaml`, then install the chart from a
clone of this repository:

```bash
helm install toolbox ./charts/mcp-toolbox \
  --namespace toolbox --create-namespace \
  --set-file config.toolsYaml=tools.yaml
```

Check that it is running:

```bash
kubectl port-forward --namespace toolbox svc/toolbox-mcp-toolbox 5000:5000
curl http://127.0.0.1:5000/healthz
```

The MCP endpoint is served at `/mcp`.

## Tool configuration

Set one of the following:

| Option | Use when |
| --- | --- |
| `config.toolsYaml` | You want the chart to store `tools.yaml` in a Secret it manages. Changing it rolls the pods. |
| `config.existingSecret` | You manage the Secret yourself (for example with External Secrets or Sealed Secrets). Updates are reloaded without a restart. |
| `config.prebuilt` | You only need [prebuilt tools](https://mcp-toolbox.dev/documentation/configuration/prebuilt-configs/). Can be combined with either option above. |

`config.toolsYaml` and `config.existingSecret` are mutually exclusive.

Values in `tools.yaml` can reference environment variables (`${DB_PASSWORD}`).
Supply them from a Secret with `envFrom`:

```yaml
envFrom:
  - secretRef:
      name: toolbox-db-credentials
```

### Reloading configuration

The tools file is mounted as a directory rather than with `subPath`, so
Kubernetes propagates Secret updates into the pod. The kubelet applies these
updates by swapping a symlink, which Toolbox's file watcher does not detect, so
the chart passes `--poll-interval` (`config.pollInterval`, default 10 seconds).
Expect a change to take effect within about a minute and a half; most of that
is the kubelet's own sync period.

## Security

The defaults meet the
[`restricted` Pod Security Standard](https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted):
non-root UID 65532 (the image's distroless `nonroot` user), read-only root
filesystem, all capabilities dropped, `RuntimeDefault` seccomp, and no
ServiceAccount token mounted.

Toolbox accepts any `Host` header and CORS origin by default. Before exposing
it outside the cluster, set the names clients will use:

```yaml
server:
  allowedHosts: [toolbox.example.com]
  allowedOrigins: [https://app.example.com]
```

`/healthz` is exempt from the host check, so probes keep working.

## Google Cloud (Workload Identity)

To let Toolbox authenticate to Google Cloud databases as a Google service
account on GKE, annotate the Kubernetes ServiceAccount and bind it as described
in the [Kubernetes deployment guide](https://mcp-toolbox.dev/documentation/deploy-to/kubernetes/):

```yaml
serviceAccount:
  annotations:
    iam.gke.io/gcp-service-account: toolbox@PROJECT_ID.iam.gserviceaccount.com
```

## Scaling

The streamable HTTP endpoint (`/mcp`) is stateless, so replicas can sit behind
the Service without session affinity. If clients use the legacy `/sse`
transport with more than one replica, set `service.sessionAffinity: ClientIP`.

`autoscaling.enabled` creates a HorizontalPodAutoscaler, and
`podDisruptionBudget.enabled` creates a PodDisruptionBudget.

## Values

| Key | Default | Description |
| --- | --- | --- |
| `image.repository` | `us-central1-docker.pkg.dev/database-toolbox/toolbox/toolbox` | Image repository. |
| `image.tag` | `""` | Image tag. Defaults to the chart `appVersion`. |
| `config.toolsYaml` | `""` | Inline `tools.yaml` content. |
| `config.existingSecret` | `""` | Existing Secret holding `tools.yaml`. |
| `config.secretKey` | `tools.yaml` | Key of the tools file in the Secret. |
| `config.prebuilt` | `[]` | Prebuilt tool sets (`--prebuilt`). |
| `config.disableReload` | `false` | Disable dynamic reloading of the tools file. |
| `config.pollInterval` | `10` | Seconds between checks for tools file changes. `0` disables polling. |
| `server.port` | `5000` | Container port. |
| `server.allowedHosts` | `[]` | Allowed `Host` headers. Empty keeps the Toolbox default (`*`). |
| `server.allowedOrigins` | `[]` | Allowed CORS origins. Empty keeps the Toolbox default (`*`). |
| `server.logLevel` | `""` | `DEBUG`, `INFO`, `WARN` or `ERROR`. |
| `server.loggingFormat` | `""` | `standard` or `JSON`. |
| `server.extraArgs` | `[]` | Additional Toolbox flags. |
| `env` / `envFrom` | `[]` | Container environment. |
| `extraVolumes` / `extraVolumeMounts` | `[]` | Additional volumes, such as database TLS certificates. |
| `serviceAccount.create` | `true` | Create a ServiceAccount. |
| `serviceAccount.annotations` | `{}` | ServiceAccount annotations. |
| `automountServiceAccountToken` | `false` | Mount a ServiceAccount token. Toolbox does not call the Kubernetes API. |
| `replicaCount` | `1` | Replicas when autoscaling is disabled. |
| `service.type` | `ClusterIP` | Service type. |
| `service.port` | `5000` | Service port. |
| `service.sessionAffinity` | `None` | Set to `ClientIP` for the legacy `/sse` transport with multiple replicas. |
| `ingress.enabled` | `false` | Create an Ingress. |
| `autoscaling.enabled` | `false` | Create a HorizontalPodAutoscaler. |
| `podDisruptionBudget.enabled` | `false` | Create a PodDisruptionBudget. |
| `resources` | `{}` | Container resource requests and limits. |

See [`values.yaml`](values.yaml) for every option.

## Development

```bash
helm lint charts/mcp-toolbox --set config.prebuilt={postgres}
helm unittest charts/mcp-toolbox
```

Tests use the [helm-unittest](https://github.com/helm-unittest/helm-unittest)
plugin.
