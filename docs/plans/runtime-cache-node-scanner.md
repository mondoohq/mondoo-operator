# Runtime Cache Node Scanner Operator Plan

## Scope

Add Mondoo Operator support for a node-local container image cache scanner. This plan belongs in `mondoo-operator` because it defines the Kubernetes API, DaemonSet/CronJob shape, RBAC, runtime socket delegation list, resource controls, and rollout behavior. MQL resources and cnspec policy content are covered in separate PRs.

## Goals

- Run image-cache scanning on every node with a small, predictable footprint.
- Scan images that are present in node runtime caches and in use by pods.
- Avoid registry pulls and avoid reading registry secrets.
- Support protected registries by scanning the already pulled image content from the local runtime.
- Provide a central, auditable runtime delegation list.
- Reuse existing scanner patterns: generated inventory ConfigMaps, Mondoo config projection, resource defaults, `GOMEMLIMIT`, and operator-managed workloads.

## Non-goals

- Do not replace the existing scheduled container-image CronJob in the first implementation.
- Do not scan images before admission or block workloads.
- Do not auto-mount every possible host path. Runtime access must be configured and explicit.
- Do not read Kubernetes `imagePullSecrets`.

## Current implementation anchors

- `controllers/container_image/resources.go` creates the current scheduled container image scan CronJob.
- `controllers/nodes/resources.go` supports node scanning as CronJob, Deployment, or DaemonSet and already handles host mounts, tolerations, and `GOMEMLIMIT`.
- `pkg/utils/gomemlimit` computes Go memory limits from Kubernetes resources.
- `pkg/utils/k8s/resources_requirements.go` contains default resource patterns.
- `api/v1alpha2/mondooauditconfig_types.go` is the CRD extension point.
- `charts/mondoo-operator/values.yaml` and generated CRDs need matching updates.

## API proposal

Add an additive runtime-cache section under `spec.containers`. Keep the existing `containers.enable` and scheduled scan behavior unchanged.

```yaml
spec:
  containers:
    enable: true
    runtimeCache:
      enable: true
      mode: daemonset
      scanOnlyInUse: true
      allowPull: false
      intervalTimer: 14400
      maxConcurrentImageScans: 1
      maxConcurrentLayerReaders: 2
      resources:
        requests:
          cpu: 25m
          memory: 128Mi
        limits:
          memory: 256Mi
      nodeSelector: {}
      tolerations: []
      delegates:
        - name: containerd-cri
          kind: containerd
          endpoint: unix:///run/containerd/containerd.sock
          hostPath: /run/containerd/containerd.sock
          priority: 10
          namespaces: ["k8s.io"]
          readOnly: true
```

Validation:

- `runtimeCache.allowPull` must default to false. If the field is retained, validation should reject true until a separate design approves it.
- `mode` initially supports `daemonset`; reserve `cronjob` for future node-targeted jobs.
- `hostPath` must be absolute.
- Delegate names must be unique.
- This draft validates `containerd` delegates only. `cri`, `crio`, `docker`, and `podman` are reserved enum values for follow-up scanner clients and fail closed for now.
- `maxConcurrentImageScans` and `maxConcurrentLayerReaders` must be at least 1.

## Workload design

Create a new controller package or subpackage under `controllers/container_image`:

- `controllers/container_image/runtime_cache/resources.go`
- `controllers/container_image/runtime_cache/resources_test.go`
- `controllers/container_image/runtime_cache/conditions.go`

Generated resources:

- One DaemonSet per `MondooAuditConfig` when `spec.containers.runtimeCache.enable=true`.
- One ConfigMap containing the inventory template and runtime delegation list.
- Reuse the Mondoo config Secret projection used by existing scanner jobs.
- Use `spec.containers.runtimeCache.serviceAccountName` for the DaemonSet. The default is `mondoo-operator-runtime-cache-scanning`; Helm exposes `runtimeCacheScanning.serviceAccount.name` with the same default so custom release names still create and bind the service account the controller selects.

DaemonSet command:

```text
cnspec serve
  --config /etc/opt/mondoo/mondoo.yml
  --inventory-file /tmp/mondoo-runtime-cache-inventory.yml
  --timer <intervalTimer converted from seconds to minutes> # when supported by the selected scanner image
```

Environment:

- `MONDOO_AUTO_UPDATE=false`
- `MONDOO_TMP_DIR=/tmp`
- `NODE_NAME` from `spec.nodeName`
- `GOMEMLIMIT` from configured resources
- Existing feature flags from `feature_flags.AllFeatureFlagsAsEnv()`
- Proxy variables only when `SkipProxyForCnspec=false`
- A lightweight init container renders the ConfigMap templates into `/tmp/mondoo-runtime-cache-inventory.yml` and `/tmp/mondoo-runtime-cache/delegates.yml`, replacing the node-name placeholder before `cnspec serve` starts.

Volumes:

- Projected config volume for Mondoo config and inventory template.
- `emptyDir` for `/tmp` with an optional `sizeLimit`.
- One hostPath volume per enabled runtime delegate socket.
- Optional runtime content-store hostPath only when the selected MQL implementation requires direct content reads. Prefer socket/API access first.

Security context:

- `allowPrivilegeEscalation=false`.
- Drop all Linux capabilities by default.
- `readOnlyRootFilesystem=true`.
- Run as UID 0 by default because common containerd sockets are root-owned. Keep all Linux capabilities dropped, privilege escalation disabled, and the root filesystem read-only.
- `privileged=false` by default. Only support privileged as an explicit escape hatch if OpenShift or runtime permissions require it.

Runtime socket access is node-level privilege. A read-only hostPath mount and `readOnly: true` delegate configuration prevent accidental file writes and tell cnspec not to pull images, but a Unix runtime socket still exposes the runtime API surface allowed by the daemon. Treat the runtime-cache DaemonSet like other node agents with host runtime access, keep its service account minimal, and prefer an image-service-only socket proxy once one is available.

## Runtime delegation ConfigMap

The operator should render one delegation list consumed by the MQL provider. It must include only configured delegates and node-independent settings.

Example inventory fragment:

```yaml
runtimeImageCache:
  nodeName: ${NODE_NAME}
  allowPull: false
  scanOnlyInUse: true
  maxConcurrentImageScans: 1
  maxConcurrentLayerReaders: 2
  delegates:
    - id: containerd-cri
      kind: containerd
      endpoint: unix:///host/run/containerd/containerd.sock
      priority: 10
      namespaces: ["k8s.io"]
      readonly: true
```

Rules:

- The path inside the scanner container may differ from host path; render the container path in `endpoint`.
- Do not render registry credentials.
- Do not mount or read image pull secrets.
- Include all delegates even if a node does not have that socket. The scanner reports per-node delegate status.

## RBAC

Runtime-cache scanning should need:

- `get`, `list`, `watch` on Pods to find images in use on the node.
- `get`, `list`, `watch` on Namespaces so scanned assets can retain namespace context.
- `get`, `list`, `watch` on Nodes to map node names, labels, taints, and addresses.
- Existing Mondoo result submission access through the mounted Mondoo config.

It should not need:

- `get`, `list`, or `watch` on Secrets.
- AdmissionReview permissions.
- Mutating webhook permissions.

## Resource management

Defaults should be conservative:

- CPU request: `25m`.
- Memory request: `128Mi`.
- Memory limit: `256Mi`.
- `maxConcurrentImageScans=1`.
- `maxConcurrentLayerReaders=2`.
- `emptyDir.sizeLimit` default: `1Gi` if temporary OCI exports are required; expose it as `spec.containers.runtimeCache.tempStorageSize` for larger cached images.

Implementation requirements:

- Set `GOMEMLIMIT` from resource limits like node scanning does.
- Pass scan concurrency values through inventory rather than command-line flags if they are provider options.
- Use DaemonSet rolling update with `maxUnavailable=10%` or 1.
- Honor `nodeSelector`, `affinity`, and `tolerations` so clusters can limit rollout during preview.

## Status and conditions

Runtime-cache scanning uses the `RuntimeCacheScanningDegraded` condition on `MondooAuditConfig` status.

Implemented condition reasons:

- `InvalidDelegateConfig`
- `RuntimeCacheReconcileFailed`
- `RuntimeCacheScanningAvailable`
- `RuntimeCacheScanningDisabled`
- `RuntimeCacheScanningUnavailable`

The scanner itself should report per-node delegate failures as asset data. Operator conditions should only represent deployment-level health.

## Rollout phases

### Phase 1: API and rendering

- Add `RuntimeCache` structs to `api/v1alpha2`.
- Regenerate deepcopy and CRDs with `make generate manifests`.
- Add Helm values and chart README entries.
- Render the runtime delegation inventory ConfigMap.
- Unit-test validation and rendering.

### Phase 2: DaemonSet resources

- Add DaemonSet builder and tests.
- Mount runtime socket hostPaths from delegates.
- Add resource defaults and `GOMEMLIMIT`.
- Add status conditions.
- Ensure existing container-image CronJob remains unchanged when runtime cache is disabled.

### Phase 3: Controller reconciliation

- Reconcile runtime-cache DaemonSet and ConfigMap when enabled.
- Delete them when disabled.
- Keep legacy container-image scan resources independent.
- Add owner references and labels consistent with existing scanner resources.

### Phase 4: Integration tests

- Add a k3d/containerd integration test that deploys a workload from a private or locally loaded image.
- Verify runtime-cache scanner scans the image without any registry pull secret.
- Verify no Secret RBAC is granted to the scanner service account.
- Verify missing runtime socket produces a degraded asset status without reconcile failure.

## Test plan

Focused tests:

- `go test ./api/...`
- `go test ./controllers/container_image/...`
- `go test ./controllers/nodes/...`
- `go test ./pkg/utils/...`
- New tests for DaemonSet resource shape, hostPath mounts, projected inventory, env vars, resource defaults, and conditions.
- New tests that rendered RBAC does not include Secret permissions for runtime-cache scanning.

Full verification:

- `make manifests generate fmt vet`
- `make test`
- `make test/integration` with `K8S_DISTRO=k3d` for the containerd path.
- `helm lint charts/mondoo-operator` through the repo's `make lint` path.

## Acceptance criteria

- Users can enable a node-local runtime-cache scanner as a DaemonSet.
- The scanner runs with bounded resources and per-node runtime delegation.
- Protected-registry images already present on nodes can be scanned without mounting image pull secrets.
- No new image pulls are initiated by the runtime-cache scanner.
- Existing scheduled container-image scanning continues to work unchanged.
- Tests cover API generation, workload rendering, RBAC, status conditions, and a k3d/containerd integration path.
