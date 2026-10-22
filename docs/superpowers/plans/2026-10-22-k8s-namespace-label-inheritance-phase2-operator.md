# K8s namespace label inheritance, Phase 2 (mondoo-operator) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A new `spec.inheritNamespaceLabels` list on `MondooAuditConfig` reaches the k8s provider as the `inherit-namespace-labels` inventory option on every cnspec scan the operator runs: the scheduled Kubernetes resources scan, external clusters, the container image scan, and the resource watcher.

**Architecture:** One helper in `pkg/utils/k8s` turns the list into the provider option (empty list → no option), the same shape as `LabelSelectorOptions`. Each of the three inventory builders merges that map into its connection options next to the label-selector options it already merges. The resource watcher gets the list as a `--inherit-namespace-labels` Deployment argument, parses it into `ScannerConfig`, and its per-batch inventory merges the same helper's output. Generated deepcopy and CRD files are regenerated and committed.

**Tech Stack:** Go, kubebuilder/controller-gen (`make generate manifests helm/crds`), testify, the operator's existing inventory builder tests.

**Spec:** mql repo `docs/superpowers/specs/2026-10-20-k8s-namespace-label-inheritance-design.md` (D1, D2 "consequence", Phase 2) and mql ADR 051 `docs/adr/051-k8s-namespace-label-inheritance.md` ("Consequences": every scanner path the operator runs must pass the option). The provider side merged as mondoohq/mql#12086.

## Global Constraints

- CRD field: `InheritNamespaceLabels []string` with json tag `inheritNamespaceLabels,omitempty`, `// +optional`, on `MondooAuditConfigSpec` directly after `Annotations`. One global list; no per-external-cluster override in this phase.
- Provider option key: `inherit-namespace-labels`; constant `InheritNamespaceLabelsOption` in `pkg/utils/k8s`. Value: the entries joined with `,`, after trimming whitespace and dropping empty entries. No option at all when nothing remains, so an unset field leaves every inventory byte-identical to today.
- All four cnspec paths get the option: `controllers/k8s_scan` `Inventory()` and `ExternalClusterInventory()`, `controllers/container_image` `containerImageOptions()`, and the resource watcher (Deployment args → cmd flag → `ScannerConfig` → scanner inventory). Missing the watcher means its rescans strip the inherited labels (labels are replaced per sync).
- No validation of glob syntax in the operator; the provider validates and fails the scan with an error naming the option, like the `images` filters.
- No remote-managed mapping (`pkg/utils/k8s/effective_config.go`): remote-managed CRs rebuild the spec from the server and will not carry this field until the Console/server integration exposes it (design Phase 4). The docs say so.
- Generated files are regenerated with `make generate manifests helm/crds` and committed in the same commit as the type change: `api/v1alpha2/zz_generated.deepcopy.go`, `config/crd/bases/k8s.mondoo.com_mondooauditconfigs.yaml`, and the chart copies under `charts/mondoo-operator/crds/`, `charts/mondoo-operator/files/crds/`, `charts/mondoo-operator/templates/`.
- Version floor in docs: the k8s provider release that includes mondoohq/mql#12086, i.e. the first k8s provider release after 14.2.0. At execution time check `git show origin/main:providers/k8s/config/config.go | grep Version:` in the mql checkout and `gh release list --repo mondoohq/mql --limit 20 | grep k8s`; if a release carrying the change exists, name it (`k8s provider X.Y.Z or later`), otherwise write "the first k8s provider release after 14.2.0 (mondoohq/mql#12086)".
- Commits: conventional style as the repo uses (`feat(scope): ...`, `docs: ...`), GPG-signed (`git config commit.gpgsign` is `true` here). No unrelated changes.
- Before Task 1, bring the worktree branch up to date with `main` (it is behind) using the app's `sync_with_base_branch` tool, then run the Task 6 test command once to confirm a green baseline.
- Tests: `go test ./pkg/utils/k8s/... ./controllers/k8s_scan/... ./controllers/container_image/... ./controllers/resource_watcher/... ./api/... -count=1`. Never `go test ./...` or `go vet ./...` over the whole repo (integration tests need a cluster); vet the same package list.

## Review Focus

1. A list entry with surrounding whitespace, or an empty string, in `inheritNamespaceLabels` → trimmed / dropped; a list of only empty strings produces no option. Pinned in Task 1 (`TestInheritNamespaceLabelsOptions`).
2. An external cluster with its own `filtering` block still gets the global `inheritNamespaceLabels` (the field is not part of `Filtering`). Pinned in Task 2 (`TestExternalClusterInventory_InheritsGlobalInheritNamespaceLabels`).
3. The resource watcher Deployment carries `--inherit-namespace-labels <csv>` and its scanner inventory carries the option, so watcher rescans keep the label. Pinned in Task 4 (both tests).
4. The container image scan inventory carries the option, so the Phase 3 provider change is picked up with no further operator change. Pinned in Task 3.
5. An existing CR without the field round-trips unchanged: the field is optional and omitted when empty. Pinned in Task 1 (CRD yaml check: the property is listed and not under `required`) and in every "Without" test.

---

### Task 1: CRD field, generated files, and the option helper

**Files:**
- Modify: `api/v1alpha2/mondooauditconfig_types.go:39-42` (after the `Annotations` field)
- Generated (commit): `api/v1alpha2/zz_generated.deepcopy.go`, `config/crd/bases/k8s.mondoo.com_mondooauditconfigs.yaml`, `charts/mondoo-operator/crds/k8s.mondoo.com_mondooauditconfigs.yaml`, `charts/mondoo-operator/files/crds/k8s.mondoo.com_mondooauditconfigs.yaml`, `charts/mondoo-operator/templates/mondooauditconfig-crd.yaml`
- Create: `pkg/utils/k8s/inherit_labels.go`
- Test: `pkg/utils/k8s/inherit_labels_test.go`

**Interfaces:**
- Produces, used by Tasks 2–4:
  ```go
  // InheritNamespaceLabelsOption is the k8s provider inventory option (and resource
  // watcher flag) naming the namespace label keys workloads inherit.
  const InheritNamespaceLabelsOption = "inherit-namespace-labels"

  // InheritNamespaceLabelsOptions converts spec.inheritNamespaceLabels into the
  // provider option. Entries are trimmed and empty ones dropped; with nothing left
  // the result is an empty map, so callers can maps.Copy it unconditionally.
  func InheritNamespaceLabelsOptions(keys []string) map[string]string
  ```
- Produces: `v1alpha2.MondooAuditConfigSpec.InheritNamespaceLabels []string`.

- [ ] **Step 1: Write the failing test** (`pkg/utils/k8s/inherit_labels_test.go`, same header and package as `label_selectors_test.go`)

```go
func TestInheritNamespaceLabelsOptions(t *testing.T) {
	tests := []struct {
		name     string
		keys     []string
		expected map[string]string
	}{
		{name: "nil", keys: nil, expected: map[string]string{}},
		{name: "empty entries only", keys: []string{"", "  "}, expected: map[string]string{}},
		{name: "keys and globs, trimmed", keys: []string{"my.company.com/team", " my.company.com/* ", ""},
			expected: map[string]string{InheritNamespaceLabelsOption: "my.company.com/team,my.company.com/*"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, InheritNamespaceLabelsOptions(tt.keys))
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/utils/k8s/ -run TestInheritNamespaceLabelsOptions -count=1`
Expected: FAIL to compile, undefined `InheritNamespaceLabelsOptions`.

- [ ] **Step 3: Implement the helper in `pkg/utils/k8s/inherit_labels.go`**

License header `// Copyright Mondoo, Inc. 2026` / `// SPDX-License-Identifier: BUSL-1.1`, package `k8s`. `strings.TrimSpace` each entry, skip empties, `strings.Join(kept, ",")`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/utils/k8s/ -count=1`
Expected: PASS (new and existing tests).

- [ ] **Step 5: Add the CRD field**

In `MondooAuditConfigSpec`, directly after `Annotations`:

```go
	// InheritNamespaceLabels lists namespace label keys that every workload discovered in a
	// namespace inherits from that namespace, for example "my.company.com/team". Glob patterns
	// are allowed ("my.company.com/*"). A label the workload already carries is kept. Applies to
	// the Kubernetes resources scan, external clusters, the container image scan and the
	// resource watcher. Requires a cnspec image whose k8s provider supports it.
	// +optional
	InheritNamespaceLabels []string `json:"inheritNamespaceLabels,omitempty"`
```

- [ ] **Step 6: Regenerate and verify the generated files**

Run: `make generate manifests helm/crds`
Then: `grep -n "inheritNamespaceLabels" config/crd/bases/k8s.mondoo.com_mondooauditconfigs.yaml charts/mondoo-operator/crds/k8s.mondoo.com_mondooauditconfigs.yaml charts/mondoo-operator/files/crds/k8s.mondoo.com_mondooauditconfigs.yaml charts/mondoo-operator/templates/mondooauditconfig-crd.yaml | wc -l` and `grep -n "InheritNamespaceLabels" api/v1alpha2/zz_generated.deepcopy.go`
Expected: four CRD files each mention the property (count ≥ 4); the deepcopy copies the slice; `git status --short` shows only the files this task names.

- [ ] **Step 7: Run the api package tests and commit**

Run: `go test ./api/... ./pkg/utils/k8s/... -count=1`
Expected: PASS.

```bash
git add api/v1alpha2/mondooauditconfig_types.go api/v1alpha2/zz_generated.deepcopy.go config/crd/bases/k8s.mondoo.com_mondooauditconfigs.yaml charts/mondoo-operator/crds/ charts/mondoo-operator/files/crds/ charts/mondoo-operator/templates/mondooauditconfig-crd.yaml pkg/utils/k8s/inherit_labels.go pkg/utils/k8s/inherit_labels_test.go
git commit -m "feat(api): add spec.inheritNamespaceLabels and the inherit-namespace-labels option helper"
```

---

### Task 2: Kubernetes resources scan and external cluster inventories

**Files:**
- Modify: `controllers/k8s_scan/resources.go:1054-1058` (`Inventory()`, right after `selectorOpts` is built) and `:1141-1145` (`ExternalClusterInventory()`, right after `maps.Copy(opts, selectorOpts)`)
- Test: `controllers/k8s_scan/resources_test.go` (next to `TestInventory_WithLabelSelectors`, l.160)

**Interfaces:**
- Consumes: `k8s.InheritNamespaceLabelsOptions`, `k8s.InheritNamespaceLabelsOption`, `Spec.InheritNamespaceLabels` (Task 1).

- [ ] **Step 1: Write the failing tests** (pattern: `TestInventory_WithLabelSelectors` / `TestExternalClusterInventory_UsesClusterLabelSelectors` in the same file)

```go
func TestInventory_WithInheritNamespaceLabels(t *testing.T) {
	auditConfig := *testAuditConfig()
	auditConfig.Spec.InheritNamespaceLabels = []string{"my.company.com/team", "my.company.com/*"}
	// Inventory(...) as in TestInventory_WithLabelSelectors; then:
	assert.Equal(t, "my.company.com/team,my.company.com/*", options[k8s.InheritNamespaceLabelsOption])
}

func TestInventory_WithoutInheritNamespaceLabels(t *testing.T) {
	// testAuditConfig() unchanged → assert.NotContains(t, options, k8s.InheritNamespaceLabelsOption)
}

func TestExternalClusterInventory_InheritsGlobalInheritNamespaceLabels(t *testing.T) {
	// auditConfig with Spec.InheritNamespaceLabels = []string{"my.company.com/team"} AND an external
	// cluster that sets its own Filtering block (copy the cluster setup from
	// TestExternalClusterInventory_UsesClusterLabelSelectors); build with ExternalClusterInventory
	// the way that test does; assert options[k8s.InheritNamespaceLabelsOption] == "my.company.com/team"
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./controllers/k8s_scan/ -run 'InheritNamespaceLabels' -count=1`
Expected: the two "With" tests FAIL on the missing option; the "Without" test passes (pins today's output).

- [ ] **Step 3: Merge the option in both builders**

`Inventory()`: after the `selectorOpts` error check, `maps.Copy(selectorOpts, k8s.InheritNamespaceLabelsOptions(m.Spec.InheritNamespaceLabels))`. `ExternalClusterInventory()`: after `maps.Copy(opts, selectorOpts)`, `maps.Copy(opts, k8s.InheritNamespaceLabelsOptions(m.Spec.InheritNamespaceLabels))`.

- [ ] **Step 4: Run the package tests**

Run: `go test ./controllers/k8s_scan/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controllers/k8s_scan/resources.go controllers/k8s_scan/resources_test.go
git commit -m "feat(k8s-scan): pass inherit-namespace-labels to the resources and external cluster scans"
```

---

### Task 3: Container image scan inventory

**Files:**
- Modify: `controllers/container_image/resources.go:425-443` (`containerImageOptions`)
- Test: `controllers/container_image/resources_test.go` (next to `TestInventory_WithAnnotations`, l.40)

**Interfaces:**
- Consumes: Task 1 helper and field.

- [ ] **Step 1: Write the failing test** (build the audit config and call `Inventory("", testClusterUID, auditConfig, v1alpha2.MondooOperatorConfig{}, nil, nil)` exactly as `TestInventory_WithAnnotations` does)

```go
func TestInventory_WithInheritNamespaceLabels(t *testing.T) {
	// Spec.InheritNamespaceLabels = []string{"my.company.com/team"}
	// options := inv.Spec.Assets[0].Connections[0].Options
	// assert.Equal(t, "my.company.com/team", options[k8s.InheritNamespaceLabelsOption])
}

func TestInventory_WithoutInheritNamespaceLabels(t *testing.T) {
	// no field → assert.NotContains(t, options, k8s.InheritNamespaceLabelsOption)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./controllers/container_image/ -run 'InheritNamespaceLabels' -count=1`
Expected: the "With" test FAILS on the missing option.

- [ ] **Step 3: Merge the option** in `containerImageOptions` after `maps.Copy(opts, selectorOpts)`: `maps.Copy(opts, k8s.InheritNamespaceLabelsOptions(m.Spec.InheritNamespaceLabels))`.

- [ ] **Step 4: Run the package tests**

Run: `go test ./controllers/container_image/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controllers/container_image/resources.go controllers/container_image/resources_test.go
git commit -m "feat(container-image): pass inherit-namespace-labels to the container image scan"
```

---

### Task 4: Resource watcher

**Files:**
- Modify: `controllers/resource_watcher/resources.go:93-102` (Deployment args loop over selector options)
- Modify: `cmd/mondoo-operator/resource_watcher/cmd.go:53-67` (flags) and `:229-240` (`ScannerConfig` construction)
- Modify: `controllers/resource_watcher/scanner.go:27-44` (`ScannerConfig`) and `:160-176` (inventory options)
- Test: `controllers/resource_watcher/resources_test.go` (next to `TestDeployment`, l.44), `controllers/resource_watcher/scanner_test.go` (next to `TestScannerGenerateInventory_LabelSelectors`, l.17)

**Interfaces:**
- Consumes: Task 1 helper and field.
- Produces: `ScannerConfig.InheritNamespaceLabels []string`; Deployment arg `--inherit-namespace-labels <csv>`; cmd flag `--inherit-namespace-labels` (StringSlice).

- [ ] **Step 1: Write the failing tests**

```go
// resources_test.go — build the Deployment the way TestDeployment does, with
// Spec.InheritNamespaceLabels = []string{"my.company.com/team", "my.company.com/*"}
func TestDeployment_WithInheritNamespaceLabels(t *testing.T) {
	// cmdStr := strings.Join(container.Command, " ") (or Args, whichever TestDeployment joins)
	assert.Contains(t, cmdStr, "--inherit-namespace-labels my.company.com/team,my.company.com/*")
}

// scanner_test.go — same table shape as TestScannerGenerateInventory_LabelSelectors
func TestScannerGenerateInventory_InheritNamespaceLabels(t *testing.T) {
	// case "set": ScannerConfig{InheritNamespaceLabels: []string{"my.company.com/team"}}
	//   → opts[k8s.InheritNamespaceLabelsOption] == "my.company.com/team"
	// case "unset": ScannerConfig{} → assert.NotContains(t, opts, k8s.InheritNamespaceLabelsOption)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./controllers/resource_watcher/ -run 'InheritNamespaceLabels' -count=1`
Expected: FAIL to compile on the missing `ScannerConfig` field, then the Deployment test fails on the missing argument.

- [ ] **Step 3: Implement**

- `scanner.go`: add `InheritNamespaceLabels []string` to `ScannerConfig` with a doc comment; in the inventory options, after the object-label-selector block: `maps.Copy(opts, k8s.InheritNamespaceLabelsOptions(s.config.InheritNamespaceLabels))`.
- `resources.go` (Deployment): after the selector-options loop, `for opt, v := range k8s.InheritNamespaceLabelsOptions(m.Spec.InheritNamespaceLabels) { cmd = append(cmd, "--"+opt, v) }` (a single-key map, so ordering is stable).
- `cmd.go`: `inheritNamespaceLabels := Cmd.Flags().StringSlice("inherit-namespace-labels", nil, "Namespace label keys (comma-separated, glob supported) that scanned workloads inherit from their namespace.")` and `InheritNamespaceLabels: *inheritNamespaceLabels` in the `ScannerConfig` literal.

- [ ] **Step 4: Run the package tests and vet the cmd package**

Run: `go test ./controllers/resource_watcher/ -count=1 && go vet ./cmd/mondoo-operator/resource_watcher/`
Expected: PASS, vet silent.

- [ ] **Step 5: Commit**

```bash
git add controllers/resource_watcher/resources.go controllers/resource_watcher/resources_test.go controllers/resource_watcher/scanner.go controllers/resource_watcher/scanner_test.go cmd/mondoo-operator/resource_watcher/cmd.go
git commit -m "feat(resource-watcher): pass inherit-namespace-labels through the watcher"
```

---

### Task 5: Documentation

**Files:**
- Modify: `docs/user-manual.md:16-17` (TOC) and `:296` (new `###` section between "Filter Kubernetes objects based on labels" and "Tune the network inventory")
- Modify: `config/samples/k8s_v1alpha2_mondooauditconfig.yaml` (commented example directly after the `filtering` block, l.~240)
- Modify: `README.md:141` (feature table row after "Namespace Filtering")

**Interfaces:** none.

- [ ] **Step 1: User manual section** `### Inherit namespace labels onto workloads`, written in the voice of the label-selector section above it, covering: what the field does with a YAML example (`inheritNamespaceLabels: ["my.company.com/team"]` plus a glob example); that the workload's own label wins and nothing is removed; the routing use case in two sentences (org-scoped scans, a `LABEL my.company.com/team` rule per team space, a catch-all for workloads in unlabeled namespaces); which scans it applies to (all four, naming the resource watcher explicitly); that container images are not labeled yet; the version floor per Global Constraints; a `> **Note:**` that remote-managed configurations do not pick this field up from the CR until it is available in the Console. Add the TOC entry under the "Filter Kubernetes objects based on labels" line.

- [ ] **Step 2: Sample CR**: a commented block after `filtering`:

```yaml
  # Copy selected namespace labels onto every workload discovered in that namespace
  # (glob patterns allowed). A label the workload already carries is kept.
  # Requires <version floor per Global Constraints>.
  # inheritNamespaceLabels:
  #   - my.company.com/team
  #   - my.company.com/*
```

- [ ] **Step 3: README** row: `| Namespace Label Inheritance      |      ✅      |           ✅            |` after the "Namespace Filtering" row, aligned with the table.

- [ ] **Step 4: Check the rendered table and commit**

Run: `grep -n "Namespace Label Inheritance" README.md && grep -n "inherit-namespace-labels\|inheritNamespaceLabels" docs/user-manual.md config/samples/k8s_v1alpha2_mondooauditconfig.yaml | wc -l`
Expected: the README row is present; at least three mentions across the manual and sample.

```bash
git add docs/user-manual.md config/samples/k8s_v1alpha2_mondooauditconfig.yaml README.md
git commit -m "docs: document spec.inheritNamespaceLabels"
```

---

### Task 6: Final verification

- [ ] **Step 1: Tests and vet on the touched packages**

Run: `go test ./pkg/utils/k8s/... ./controllers/k8s_scan/... ./controllers/container_image/... ./controllers/resource_watcher/... ./api/... -count=1 && go vet ./pkg/utils/k8s/... ./controllers/k8s_scan/... ./controllers/container_image/... ./controllers/resource_watcher/... ./api/... ./cmd/mondoo-operator/resource_watcher/`
Expected: all PASS, vet silent.

- [ ] **Step 2: Generated files are current and the diff is scoped**

Run: `make generate manifests helm/crds && git status --short`
Expected: no changes after regeneration (everything was committed in Task 1).
Run: `git diff $(git merge-base origin/main HEAD) --stat && git log origin/main..HEAD --format='%h %G? %s'`
Expected: only the files this plan names; every commit `G`.

- [ ] **Step 3: Hand off**

Report the branch and commits. Phase 3 (container images) is a separate mql plan; Phase 4 (Console/server parity, including `effective_config.go`'s remote mapping and `K8sIntegrationConfig`) is a follow-up.
