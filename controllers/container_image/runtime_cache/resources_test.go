// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package runtime_cache

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mql/v13/providers-sdk/v1/inventory"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

const testClusterUID = "cluster-uid"

func runtimeCacheAuditConfig() *v1alpha2.MondooAuditConfig {
	return &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mondoo-client",
			Namespace: "mondoo-operator",
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			MondooCredsSecretRef: corev1.LocalObjectReference{Name: "mondoo-creds"},
			Containers: v1alpha2.Containers{
				RuntimeCache: v1alpha2.RuntimeCacheScanner{
					Enable: true,
					Delegates: []v1alpha2.RuntimeCacheDelegate{
						{
							Name:       "containerd-cri",
							Kind:       v1alpha2.RuntimeCacheDelegateKind_Containerd,
							HostPath:   "/run/containerd/containerd.sock",
							Priority:   10,
							Namespaces: []string{"k8s.io"},
						},
					},
				},
			},
		},
	}
}

func TestValidateRuntimeCache(t *testing.T) {
	cfg := runtimeCacheAuditConfig().Spec.Containers.RuntimeCache
	require.NoError(t, Validate(cfg))

	cfg.AllowPull = true
	assert.ErrorContains(t, Validate(cfg), "allowPull must be false")

	cfg = runtimeCacheAuditConfig().Spec.Containers.RuntimeCache
	cfg.Delegates[0].HostPath = "relative.sock"
	assert.ErrorContains(t, Validate(cfg), "hostPath must be absolute")

	cfg = runtimeCacheAuditConfig().Spec.Containers.RuntimeCache
	cfg.Delegates[0].ReadOnly = ptr.To(false)
	assert.ErrorContains(t, Validate(cfg), "readOnly must be true")

	cfg = runtimeCacheAuditConfig().Spec.Containers.RuntimeCache
	cfg.Delegates[0].Name = strings.Repeat("a", 64)
	assert.ErrorContains(t, Validate(cfg), "must be a valid DNS label")

	cfg = runtimeCacheAuditConfig().Spec.Containers.RuntimeCache
	cfg.Delegates = append(cfg.Delegates, cfg.Delegates[0])
	assert.ErrorContains(t, Validate(cfg), "duplicate name")

	cfg = runtimeCacheAuditConfig().Spec.Containers.RuntimeCache
	cfg.Delegates[0].Kind = v1alpha2.RuntimeCacheDelegateKind_CRIO
	assert.ErrorContains(t, Validate(cfg), `kind "crio" is reserved for future support`)

	cfg = runtimeCacheAuditConfig().Spec.Containers.RuntimeCache
	cfg.Delegates = append(cfg.Delegates, v1alpha2.RuntimeCacheDelegate{
		Name:     "same-socket",
		Kind:     v1alpha2.RuntimeCacheDelegateKind_Containerd,
		HostPath: "/run/containerd/../containerd/containerd.sock",
	})
	assert.ErrorContains(t, Validate(cfg), `hostPath duplicates delegate "containerd-cri" after path cleaning`)
}

func TestValidateRuntimeCacheScannerSets(t *testing.T) {
	cfg := runtimeCacheAuditConfig().Spec.Containers.RuntimeCache
	cfg.ScannerSets = []v1alpha2.NodeScannerSet{{Name: "control-plane"}, {Name: "workers"}}
	require.NoError(t, Validate(cfg))

	cfg.ScannerSets = []v1alpha2.NodeScannerSet{{Name: ""}}
	assert.ErrorContains(t, Validate(cfg), "containers.runtimeCache.scannerSets[0].name is required")

	cfg.ScannerSets = []v1alpha2.NodeScannerSet{{Name: "Control_Plane"}}
	assert.ErrorContains(t, Validate(cfg), "must be a valid DNS label")

	cfg.ScannerSets = []v1alpha2.NodeScannerSet{{Name: strings.Repeat("a", 64)}}
	assert.ErrorContains(t, Validate(cfg), "must be a valid DNS label")

	cfg.ScannerSets = []v1alpha2.NodeScannerSet{{Name: "workers"}, {Name: "workers"}}
	assert.ErrorContains(t, Validate(cfg), "containers.runtimeCache.scannerSets[workers].name must be unique")
}

func TestDelegateConfig(t *testing.T) {
	cfg := runtimeCacheAuditConfig().Spec.Containers.RuntimeCache
	out, err := DelegateConfig(cfg)
	require.NoError(t, err)

	assert.Contains(t, out, "allowPull: false")
	assert.Contains(t, out, "scanOnlyInUse: true")
	assert.Contains(t, out, "maxConcurrentImageScans: 1")
	assert.Contains(t, out, "maxConcurrentLayerReaders: 2")
	assert.Contains(t, out, "kind: containerd")
	assert.Contains(t, out, "endpoint: unix:///host/run/containerd/containerd.sock")
	assert.Contains(t, out, "hostPath: /run/containerd/containerd.sock")
	assert.Contains(t, out, runtimeCacheNodeNamePlaceholder)
	assert.NotContains(t, out, "{{ getenv")
	assert.NotContains(t, strings.ToLower(out), "secret")
}

func TestInventoryRuntimeCacheOptions(t *testing.T) {
	m := runtimeCacheAuditConfig()
	invStr, err := Inventory("integration-mrn", testClusterUID, *m)
	require.NoError(t, err)

	var inv inventory.Inventory
	require.NoError(t, yaml.Unmarshal([]byte(invStr), &inv))
	require.Len(t, inv.Spec.Assets, 1)
	require.Len(t, inv.Spec.Assets[0].Connections, 1)

	conn := inv.Spec.Assets[0].Connections[0]
	assert.Equal(t, "k8s", conn.Type)
	assert.Equal(t, "true", conn.Options["runtime-cache"])
	assert.Equal(t, "false", conn.Options["runtime-cache-allow-pull"])
	assert.Equal(t, "true", conn.Options["runtime-cache-scan-only-in-use"])
	assert.Equal(t, runtimeCacheDelegatesRenderedPath, conn.Options["runtime-cache-delegates-file"])
	assert.Equal(t, runtimeCacheNodeNamePlaceholder, conn.Options["runtime-cache-node-name"])
	assert.Contains(t, invStr, runtimeCacheNodeNamePlaceholder)
	assert.NotContains(t, invStr, "{{ getenv")
	assert.Equal(t, "app=mondoo-runtime-cache-scan,mondoo_cr=mondoo-client,scan=runtime-cache", conn.Options["runtime-cache-scanner-pod-selector"])
	assert.Equal(t, []string{"runtime-cache-images"}, conn.Discover.Targets)
	assert.Equal(t, "integration-mrn", inv.Spec.Assets[0].Labels["mondoo.com/integration-mrn"])
}

func TestDaemonSetRuntimeCacheShape(t *testing.T) {
	m := runtimeCacheAuditConfig()
	ds, err := DaemonSet("cnspec:test", constants.BusyBoxImage, "integration-mrn", testClusterUID, m, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	assert.Equal(t, DaemonSetName(m.Name), ds.Name)
	assert.Equal(t, DefaultServiceAccount, ds.Spec.Template.Spec.ServiceAccountName)
	assert.NotNil(t, ds.Spec.UpdateStrategy.RollingUpdate)
	assert.Equal(t, intstr.FromString("10%"), *ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)
	assert.True(t, *ds.Spec.Template.Spec.AutomountServiceAccountToken)

	container := ds.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "mondoo-runtime-cache-scan", container.Name)
	assert.Contains(t, strings.Join(container.Command, " "), "--inventory-file "+runtimeCacheInventoryRenderedPath)
	assert.Contains(t, strings.Join(container.Command, " "), "--timer 240")
	assert.Equal(t, k8s.DefaultRuntimeCacheScanningResources, container.Resources)
	env := envToMap(container.Env)
	assert.Equal(t, "false", env["MONDOO_AUTO_UPDATE"])
	assert.Equal(t, "/tmp", env["MONDOO_TMP_DIR"])
	assert.NotEmpty(t, env["GOMEMLIMIT"])

	assert.False(t, *container.SecurityContext.Privileged)
	assert.False(t, *container.SecurityContext.AllowPrivilegeEscalation)
	assert.True(t, *container.SecurityContext.ReadOnlyRootFilesystem)
	assert.Equal(t, []corev1.Capability{"ALL"}, container.SecurityContext.Capabilities.Drop)

	assertVolumeMount(t, container.VolumeMounts, "runtime-containerd-cri", "/host/run/containerd/containerd.sock", true)
	assertHostPathVolume(t, ds.Spec.Template.Spec.Volumes, "runtime-containerd-cri", "/run/containerd/containerd.sock", corev1.HostPathSocket)
	assertEmptyDirVolume(t, ds.Spec.Template.Spec.Volumes, "temp", resource.MustParse("1Gi"))
	assertVolumeMount(t, container.VolumeMounts, "config", "/etc/opt/mondoo", true)

	require.Len(t, ds.Spec.Template.Spec.InitContainers, 1)
	init := ds.Spec.Template.Spec.InitContainers[0]
	assert.Equal(t, "render-runtime-cache-config", init.Name)
	assert.Equal(t, constants.BusyBoxImage, init.Image)
	initCommand := strings.Join(init.Command, " ")
	assert.Contains(t, initCommand, "/bin/sh -ec")
	assert.Contains(t, initCommand, "sed")
	assert.Contains(t, initCommand, runtimeCacheNodeNamePlaceholder)
	assert.Contains(t, initCommand, runtimeCacheInventoryTemplatePath)
	assert.Contains(t, initCommand, runtimeCacheInventoryRenderedPath)
	assert.Contains(t, initCommand, runtimeCacheDelegatesTemplatePath)
	assert.Contains(t, initCommand, runtimeCacheDelegatesRenderedPath)
	assertVolumeMount(t, init.VolumeMounts, "config", "/etc/opt/mondoo", true)
	assertVolumeMount(t, init.VolumeMounts, "temp", "/tmp", false)
}

func TestDaemonSetRuntimeCacheCustomIntervalAndTempStorage(t *testing.T) {
	m := runtimeCacheAuditConfig()
	m.Spec.Containers.RuntimeCache.IntervalTimer = 61
	tempStorageSize := resource.MustParse("4Gi")
	m.Spec.Containers.RuntimeCache.TempStorageSize = &tempStorageSize

	ds, err := DaemonSet("cnspec:test", constants.BusyBoxImage, "", testClusterUID, m, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	assert.Contains(t, strings.Join(ds.Spec.Template.Spec.Containers[0].Command, " "), "--timer 2")
	assertEmptyDirVolume(t, ds.Spec.Template.Spec.Volumes, "temp", tempStorageSize)
}

func TestDaemonSetRuntimeCacheLatestImageOmitsUnsupportedTimerFlag(t *testing.T) {
	m := runtimeCacheAuditConfig()

	ds, err := DaemonSet("ghcr.io/mondoohq/mondoo-operator/cnspec:latest", constants.BusyBoxImage, "", testClusterUID, m, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	command := strings.Join(ds.Spec.Template.Spec.Containers[0].Command, " ")
	assert.Contains(t, command, "--inventory-file "+runtimeCacheInventoryRenderedPath)
	assert.NotContains(t, command, "--inventory-template")
	assert.NotContains(t, command, "--timer")
}

func TestDaemonSetRuntimeCacheConfiguredLatestOmitsTimerForResolvedDigest(t *testing.T) {
	m := runtimeCacheAuditConfig()
	m.Spec.Scanner.Image.Tag = "latest"

	ds, err := DaemonSet("ghcr.io/mondoohq/mondoo-operator/cnspec@sha256:abc123", constants.BusyBoxImage, "", testClusterUID, m, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	command := strings.Join(ds.Spec.Template.Spec.Containers[0].Command, " ")
	assert.NotContains(t, command, "--timer")
}

func TestDaemonSetRuntimeCacheConfiguredDigestSupportsTimer(t *testing.T) {
	m := runtimeCacheAuditConfig()
	m.Spec.Scanner.Image.Tag = "latest"
	m.Spec.Scanner.Image.Digest = "sha256:abc123"

	ds, err := DaemonSet("ghcr.io/mondoohq/mondoo-operator/cnspec@sha256:abc123", constants.BusyBoxImage, "", testClusterUID, m, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	command := strings.Join(ds.Spec.Template.Spec.Containers[0].Command, " ")
	assert.Contains(t, command, "--timer 240")
}

func TestDaemonSetForScannerSetAppliesSchedulingOverrides(t *testing.T) {
	m := runtimeCacheAuditConfig()
	m.Spec.Containers.RuntimeCache.Env = []corev1.EnvVar{{Name: "SHARED", Value: "base"}}
	set := v1alpha2.NodeScannerSet{
		Name: "control-plane",
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("25m")},
			Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("128Mi")},
		},
		NodeSelector: map[string]string{"node-role.kubernetes.io/control-plane": ""},
		Tolerations: []corev1.Toleration{
			{Key: "node-role.kubernetes.io/control-plane", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
		},
		PriorityClassName: "system-cluster-critical",
		Env:               []corev1.EnvVar{{Name: "SHARED", Value: "set"}, {Name: "SET_ONLY", Value: "true"}},
	}

	ds, err := DaemonSetForScannerSet("cnspec:test", constants.BusyBoxImage, "integration-mrn", testClusterUID, m, v1alpha2.MondooOperatorConfig{}, set)
	require.NoError(t, err)

	assert.Equal(t, DaemonSetNameForScannerSet(m.Name, set.Name), ds.Name)
	assert.Equal(t, set.Name, ds.Labels["scanner_set"])
	assert.Equal(t, set.Name, ds.Spec.Template.Labels["scanner_set"])
	assert.Equal(t, set.NodeSelector, ds.Spec.Template.Spec.NodeSelector)
	assert.Equal(t, set.Tolerations, ds.Spec.Template.Spec.Tolerations)
	assert.Equal(t, set.PriorityClassName, ds.Spec.Template.Spec.PriorityClassName)
	assert.Equal(t, set.Resources, ds.Spec.Template.Spec.Containers[0].Resources)
	env := envToMap(ds.Spec.Template.Spec.Containers[0].Env)
	assert.Equal(t, "set", env["SHARED"])
	assert.Equal(t, "true", env["SET_ONLY"])
}

func TestDaemonSetForScannerSetInheritsTopLevelFields(t *testing.T) {
	m := runtimeCacheAuditConfig()
	m.Spec.Containers.RuntimeCache.NodeSelector = map[string]string{"pool": "shared"}
	m.Spec.Containers.RuntimeCache.Tolerations = []corev1.Toleration{
		{Key: "shared", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
	}
	m.Spec.Containers.RuntimeCache.PriorityClassName = "shared-priority"
	m.Spec.Containers.RuntimeCache.Env = []corev1.EnvVar{{Name: "SHARED", Value: "true"}}

	ds, err := DaemonSetForScannerSet("cnspec:test", constants.BusyBoxImage, "integration-mrn", testClusterUID, m, v1alpha2.MondooOperatorConfig{}, v1alpha2.NodeScannerSet{Name: "workers"})
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"pool": "shared"}, ds.Spec.Template.Spec.NodeSelector)
	assert.Equal(t, m.Spec.Containers.RuntimeCache.Tolerations, ds.Spec.Template.Spec.Tolerations)
	assert.Equal(t, "shared-priority", ds.Spec.Template.Spec.PriorityClassName)
	assert.Equal(t, "true", envToMap(ds.Spec.Template.Spec.Containers[0].Env)["SHARED"])
}

func TestDaemonSetDoesNotMountRegistrySecrets(t *testing.T) {
	m := runtimeCacheAuditConfig()
	ds, err := DaemonSet("cnspec:test", constants.BusyBoxImage, "", testClusterUID, m, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	podSpec := ds.Spec.Template.Spec
	for _, volume := range podSpec.Volumes {
		assert.NotEqual(t, "pull-secrets", volume.Name)
	}
	for _, mount := range podSpec.Containers[0].VolumeMounts {
		assert.NotEqual(t, "/etc/opt/mondoo/docker", mount.MountPath)
	}
	_, hasDockerConfig := envToMap(podSpec.Containers[0].Env)["DOCKER_CONFIG"]
	assert.False(t, hasDockerConfig)
}

func TestDaemonSetTolerationsAreStableAndDeduplicated(t *testing.T) {
	m := runtimeCacheAuditConfig()
	m.Spec.Containers.RuntimeCache.Tolerations = []corev1.Toleration{
		{Key: "node.kubernetes.io/not-ready", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
		{Key: "node.kubernetes.io/not-ready", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
		{Key: "runtime", Operator: corev1.TolerationOpEqual, Value: "containerd", Effect: corev1.TaintEffectNoExecute},
		{Key: "runtime", Operator: corev1.TolerationOpEqual, Value: "containerd", Effect: corev1.TaintEffectNoExecute},
	}

	ds, err := DaemonSet("cnspec:test", constants.BusyBoxImage, "", testClusterUID, m, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	assert.Equal(t, []corev1.Toleration{
		{Key: "node.kubernetes.io/not-ready", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
		{Key: "runtime", Operator: corev1.TolerationOpEqual, Value: "containerd", Effect: corev1.TaintEffectNoExecute},
	}, ds.Spec.Template.Spec.Tolerations)
}

func TestDaemonSetRuntimeDelegatesMountExactSockets(t *testing.T) {
	m := runtimeCacheAuditConfig()
	m.Spec.Containers.RuntimeCache.Delegates = append(m.Spec.Containers.RuntimeCache.Delegates, v1alpha2.RuntimeCacheDelegate{
		Name:     "containerd-debug",
		Kind:     v1alpha2.RuntimeCacheDelegateKind_Containerd,
		HostPath: "/run/containerd/containerd-debug.sock",
		Priority: 20,
	})

	ds, err := DaemonSet("cnspec:test", constants.BusyBoxImage, "", testClusterUID, m, v1alpha2.MondooOperatorConfig{})
	require.NoError(t, err)

	assertHostPathVolume(t, ds.Spec.Template.Spec.Volumes, "runtime-containerd-cri", "/run/containerd/containerd.sock", corev1.HostPathSocket)
	assertVolumeMount(t, ds.Spec.Template.Spec.Containers[0].VolumeMounts, "runtime-containerd-cri", "/host/run/containerd/containerd.sock", true)
	assertHostPathVolume(t, ds.Spec.Template.Spec.Volumes, "runtime-containerd-debug", "/run/containerd/containerd-debug.sock", corev1.HostPathSocket)
	assertVolumeMount(t, ds.Spec.Template.Spec.Containers[0].VolumeMounts, "runtime-containerd-debug", "/host/run/containerd/containerd-debug.sock", true)

	containerdMounts := 0
	for _, mount := range ds.Spec.Template.Spec.Containers[0].VolumeMounts {
		if strings.HasPrefix(mount.MountPath, "/host/run/containerd/") {
			containerdMounts++
		}
	}
	assert.Equal(t, 2, containerdMounts)
}

func TestScannerSetNameOrHashTrimsDashBeforeHash(t *testing.T) {
	name := scannerSetNameOrHash(16, "abcdef-ghijklmnopqrstuvwxyz")
	assert.Regexp(t, `^abcdef-[0-9a-f]{8}$`, name)
	assert.NotContains(t, name, "--")
}

func TestRuntimeCacheRBACDoesNotGrantSecrets(t *testing.T) {
	role, err := os.ReadFile("../../../config/rbac/runtime_cache_scanning_clusterrole.yaml")
	require.NoError(t, err)
	assert.NotContains(t, string(role), "secrets")
	assert.Contains(t, string(role), "- namespaces")
	assert.Contains(t, string(role), "- nodes")
	assert.Contains(t, string(role), "- pods")

	helmRole, err := os.ReadFile("../../../charts/mondoo-operator/templates/runtime-cache-scanning-rbac.yaml")
	require.NoError(t, err)
	assert.NotContains(t, string(helmRole), "secrets")
	assert.Contains(t, string(helmRole), "- namespaces")
	assert.Contains(t, string(helmRole), "- nodes")
	assert.Contains(t, string(helmRole), "- pods")
}

func TestDelegateVolumeNameIsStableAndBounded(t *testing.T) {
	name := DelegateVolumeName("containerd.with_symbols_and_a_very_long_name_that_needs_truncation")
	assert.LessOrEqual(t, len(name), 63)
	assert.True(t, strings.HasPrefix(name, "runtime-containerd-with-symbols"))
}

func envToMap(env []corev1.EnvVar) map[string]string {
	out := map[string]string{}
	for _, item := range env {
		out[item.Name] = item.Value
	}
	return out
}

func assertVolumeMount(t *testing.T, mounts []corev1.VolumeMount, name, mountPath string, readOnly bool) {
	t.Helper()
	for _, mount := range mounts {
		if mount.Name == name {
			assert.Equal(t, mountPath, mount.MountPath)
			assert.Equal(t, readOnly, mount.ReadOnly)
			return
		}
	}
	require.Failf(t, "volume mount not found", "name=%s", name)
}

func assertHostPathVolume(t *testing.T, volumes []corev1.Volume, name, hostPath string, hostPathType corev1.HostPathType) {
	t.Helper()
	for _, volume := range volumes {
		if volume.Name == name {
			require.NotNil(t, volume.HostPath)
			assert.Equal(t, hostPath, volume.HostPath.Path)
			assert.Equal(t, ptr.To(hostPathType), volume.HostPath.Type)
			return
		}
	}
	require.Failf(t, "hostPath volume not found", "name=%s", name)
}

func assertEmptyDirVolume(t *testing.T, volumes []corev1.Volume, name string, size resource.Quantity) {
	t.Helper()
	for _, volume := range volumes {
		if volume.Name == name {
			require.NotNil(t, volume.EmptyDir)
			require.NotNil(t, volume.EmptyDir.SizeLimit)
			assert.True(t, volume.EmptyDir.SizeLimit.Equal(size), "expected %s, got %s", size.String(), volume.EmptyDir.SizeLimit.String())
			return
		}
	}
	require.Failf(t, "emptyDir volume not found", "name=%s", name)
}
