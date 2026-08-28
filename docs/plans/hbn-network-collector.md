# HBN Network Collector Operator Plan

## Scope

Add Mondoo Operator configuration, RBAC, and scanner wiring for HBN network inventory, secondary-interface network policies, egress routing, NAT visibility, and optional flow integrations. MQL resources and cnspec policy content are covered in separate PRs.

## Implementation status

This PR implements the operator API and scanner inventory contract:

- `spec.kubernetesResources.networkInventory` CRD fields.
- Deterministic `kubernetesNetworkInventory` inventory option rendering for local and external Kubernetes scans.
- CIDR and optional flow endpoint validation before scanner ConfigMaps are written.
- Base network posture provider options for services, ingresses, native NetworkPolicy, Gateway API classes/gateways/routes/reference grants, AdminNetworkPolicy/BaselineAdminNetworkPolicy, MultiNetworkPolicy, Calico, Cilium, and HBN network resources.
- Sample configuration and focused unit coverage.

The current scanner ClusterRole in this repository is already wildcard read-only (`apiGroups: ['*']`, `resources: ['*']`, `verbs: get/list/watch`). This PR therefore does not add effective HBN-specific RBAC. A follow-up least-privilege RBAC PR should replace that wildcard role with explicit core Kubernetes, Gateway API, HBN, and MultiNetworkPolicy rules.

## Goals

- Allow the operator-managed Kubernetes scanner to collect HBN, MultiNetworkPolicy, Gateway API, AdminNetworkPolicy, Calico, and Cilium network inventory.
- Keep the feature scan-time only; do not enter admission paths.
- Preserve the current scanner RBAC surface in this PR while rendering network inventory options for the provider. Least-privilege scanner RBAC remains a follow-up because the current scanner ClusterRole is wildcard read-only.
- Make internet exposure, egress NAT, network classifications, and secondary-interface policy coverage visible in Mondoo.
- Keep Calico Whisker and Cilium flow integrations optional and disabled by default.

## Non-goals

- Do not install HBN, MultiNetworkPolicy, Calico, Cilium, or Whisker.
- Do not change cluster network policy enforcement.
- Do not require HBN CRDs for ordinary Kubernetes scans.
- Do not collect packet payloads.

## Current implementation anchors

- `controllers/k8s_scan/resources.go` renders Kubernetes resource scan workloads and inventory.
- `controllers/resource_watcher` has resource-type selection patterns but this feature should remain scheduled scan inventory unless a later PR adds watch support.
- `charts/mondoo-operator/templates/k8s-resources-scanning-rbac.yaml` and `config/rbac/k8s_resources_scanning_clusterrole.yaml` are the RBAC surfaces.
- `api/v1alpha2/mondooauditconfig_types.go` is the CRD extension point.
- `docs/operator-config.md` and `docs/user-manual.md` are the user-facing docs surfaces.

## API proposal

Add a network inventory section under `spec.kubernetesResources`.

```yaml
spec:
  kubernetesResources:
    enable: true
    networkInventory:
      enable: true
      hbn:
        enable: true
        includeLegacyResources: true
      multiNetworkPolicy:
        enable: true
      classifications:
        publicCidrs: []
        privateCidrs: []
        trustedEgressCidrs: []
      observedFlows:
        enable: false
        calicoWhisker:
          enable: false
          namespace: calico-system
          serviceName: whisker
        ciliumHubble:
          enable: false
          namespace: kube-system
          serviceName: hubble-relay
```

Defaults:

- `networkInventory.enable=false` for first release.
- `hbn.enable=true` only has effect when `networkInventory.enable=true`.
- `multiNetworkPolicy.enable=true` only has effect when `networkInventory.enable=true`.
- `observedFlows.enable=false`.

Validation:

- CIDRs must parse as IPv4 or IPv6 CIDRs.
- Whisker and Hubble namespaces must be valid Kubernetes names.
- Flow integration service names must be valid Kubernetes names.

## Inventory wiring

When enabled, render inventory options for the k8s provider:

```yaml
kubernetesNetworkInventory:
  hbn:
    enabled: true
    includeLegacyResources: true
    apiGroups:
      - network-connector.sylvaproject.org
      - network.t-caas.telekom.com
  multiNetworkPolicy:
    enabled: true
    apiGroup: k8s.cni.cncf.io
  classifications:
    publicCidrs: []
    privateCidrs: []
    trustedEgressCidrs: []
  observedFlows:
    enabled: false
```

The scanner should still perform API discovery. Operator config enables intent and RBAC; the MQL provider decides which resources exist in the target cluster.

Network inventory stays cluster-scoped in the scanner target list. Disabled
network inventory leaves the baseline target list unchanged; enabled network
inventory ensures the provider-supported `clusters` target is present and passes
native NetworkPolicy, Gateway API, AdminNetworkPolicy/BaselineAdminNetworkPolicy,
MultiNetworkPolicy, NetworkAttachmentDefinition, Calico, Cilium, and HBN intent
through `kubernetesNetworkInventory` provider options. Branches that enable other
optional scanner integrations, such as Kyverno, must compose their inventory
options through the same helper path so both feature sets can be enabled together
without dropping either set of options.

## RBAC plan

Add conditional RBAC rules for the scanner service account when network inventory is enabled.

HBN intent resources:

- API group: `network-connector.sylvaproject.org`
- Resources:
  - `vrfs`
  - `networks`
  - `destinations`
  - `layer2attachments`
  - `inbounds`
  - `outbounds`
  - `bgppeerings`
  - `collectors`
  - `trafficmirrors`
  - `nodenetworkconfigs`
  - `nodenetworkstatuses`
  - `networkconnectors`
  - `networkconnectorconfigs`
  - `announcementpolicies`
  - `interfaceconfigs`
  - `nodeattachments`
  - `podnetworks`

Legacy HBN resources:

- API group: `network.t-caas.telekom.com`
- Resources:
  - `bgppeerings`
  - `layer2networkconfigurations`
  - `networkconfigrevisions`
  - `nodenetplanconfigs`
  - `nodenetworkconfigs`
  - `vrfrouteconfigurations`

Secondary-interface policy resources:

- API group: `k8s.cni.cncf.io`
- Resources:
  - `network-attachment-definitions`
  - `multi-networkpolicies`

Admin policy resources:

- API group: `policy.networking.k8s.io`
- Resources:
  - `adminnetworkpolicies`
  - `baselineadminnetworkpolicies`

Gateway API resources:

- API group: `gateway.networking.k8s.io`
- Resources:
  - `gatewayclasses`
  - `gateways`
  - `httproutes`
  - `grpcroutes`
  - `tlsroutes`
  - `tcproutes`
  - `udproutes`
  - `referencegrants`

Calico resources:

- API group: `crd.projectcalico.org`
- Resources:
  - `networkpolicies`
  - `globalnetworkpolicies`

Cilium resources:

- API group: `cilium.io`
- Resources:
  - `ciliumnetworkpolicies`
  - `ciliumclusterwidenetworkpolicies`

Verbs:

- `get`, `list`, `watch` only.
- No `create`, `update`, `patch`, or `delete`.
- Current scanner RBAC reuses the existing Kubernetes resource scanner wildcard read role, which includes Secrets. This PR must not add write, admission, mutation, or network-enforcement permissions. Least-privilege read RBAC that excludes Secrets remains follow-up hardening.

## Workload behavior

The scheduled Kubernetes resource scanner remains the primary workload:

- Continue using `cnspec scan k8s`.
- Add network inventory options to the generated inventory.
- Reuse existing scanner image, config projection, proxy handling, and resource settings.
- Do not add a new controller process for static network inventory.

Observed flows are optional and should be added carefully:

- If enabled, scanner inventory includes Whisker or Hubble endpoint metadata.
- Collection must be bounded by time range, maximum records, and timeout.
- Flow collection failures should degrade flow evidence only, not fail Kubernetes inventory.

## Status and conditions

Network inventory uses the existing `K8sResourcesScanningDegraded` condition.
Implemented invalid-configuration reasons:

- `InvalidCIDRClassification`
- `InvalidObservedFlowConfig`
- `InvalidObservedFlowEndpoint`

Future scanner evidence or condition reasons:

- `OptionalCRDMissing`
- `RBACMissing`
- `ObservedFlowEndpointUnavailable`

Missing optional CRDs should not mark the operator unhealthy. They should appear as scanner evidence when possible.

## Rollout phases

### Phase 1: API and Helm values

- Add `NetworkInventory` structs to `api/v1alpha2`.
- Regenerate CRDs and deepcopy with `make generate manifests`.
- Add Helm values and README table entries.
- Add samples for HBN-only and HBN plus MultiNetworkPolicy.

### Phase 2: RBAC rendering

- Add conditional RBAC rules in Kustomize and Helm templates.
- Unit-test the exact rule set for enabled and disabled configurations.
- Confirm disabled config produces no HBN or MultiNetworkPolicy RBAC.

### Phase 3: Inventory rendering

- Render provider options into the Kubernetes scan inventory ConfigMap.
- Unit-test inventory output for defaults, explicit CIDR lists, flow disabled, Whisker enabled, and Hubble enabled.

### Phase 4: Integration tests

- Add envtest or fake-client tests for CRD presence and RBAC generation.
- Add k3d integration fixtures installing sample CRDs only, not full HBN agents.
- Verify a scan workload can list sample HBN, MultiNetworkPolicy, Gateway API route, AdminNetworkPolicy, Calico, and Cilium resources.

### Phase 5: Optional observed flows

- Add Whisker and Hubble endpoint config once MQL provider resources are available.
- Add timeout, max-records, and disabled-by-default tests.

## Test plan

Focused tests:

- `go test ./api/...`
- `go test ./controllers/k8s_scan/...`
- `go test ./controllers/resource_watcher/...` only if resource-type selection is touched.
- `go test ./pkg/utils/k8s/...`
- RBAC unit tests for HBN, legacy HBN, MultiNetworkPolicy, Gateway API, AdminNetworkPolicy, Calico, Cilium, disabled state, and no Secret verbs.
- Inventory ConfigMap tests for every network inventory option.

Full verification:

- `make manifests generate fmt vet`
- `make test`
- `make test/integration` with a k3d fixture that installs representative CRDs.
- `make lint` to cover Helm linting and generated artifacts.

## Acceptance criteria

- Enabling `networkInventory` renders provider options for HBN, MultiNetworkPolicy, Gateway API, AdminNetworkPolicy, Calico, and Cilium resources while keeping scanner discovery cluster-scoped and without adding write, admission, mutation, or network-enforcement permissions.
- Disabling `networkInventory` leaves existing Kubernetes scan discovery targets and RBAC unchanged.
- Disabling optional HBN or MultiNetworkPolicy sources keeps their provider option blocks present with `enabled: false`, so the scanner receives an explicit disabled-source signal.
- The generated scanner inventory carries network classification and optional flow settings.
- Clusters without HBN CRDs continue to scan successfully.
- Existing wildcard Kubernetes resource scanner read RBAC, including Secret read, remains unchanged and is tracked as a hardening gap.
