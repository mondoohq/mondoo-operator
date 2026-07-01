// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package runtime_cache

import (
	"crypto/sha256"
	"fmt"
	"path"
	"sort"
	"strings"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/feature_flags"
	"go.mondoo.com/mondoo-operator/pkg/utils/gomemlimit"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	mondoo "go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.mondoo.com/mql/v13/providers-sdk/v1/inventory"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

const (
	DaemonSetNameSuffix      = "-runtime-cache-scan"
	InventoryConfigMapSuffix = "-runtime-cache-inventory"
	DefaultServiceAccount    = "mondoo-operator-runtime-cache-scanning"

	defaultIntervalTimerSeconds = 14400

	configVolumeName = "config"
	tempVolumeName   = "temp"

	runtimeCacheInventoryTemplatePath = "/etc/opt/mondoo/runtime-cache-inventory.yml"
	runtimeCacheDelegatesTemplatePath = "/etc/opt/mondoo/runtime-cache/delegates.yml"
	runtimeCacheInventoryRenderedPath = "/tmp/mondoo-runtime-cache-inventory.yml"
	runtimeCacheDelegatesRenderedPath = "/tmp/mondoo-runtime-cache/delegates.yml"
	runtimeCacheNodeNamePlaceholder   = "__MONDOO_RUNTIME_CACHE_NODE_NAME__"
)

type delegateConfigFile struct {
	RuntimeImageCache runtimeImageCacheConfig `json:"runtimeImageCache"`
}

type runtimeImageCacheConfig struct {
	NodeName                  string                  `json:"nodeName"`
	AllowPull                 bool                    `json:"allowPull"`
	ScanOnlyInUse             bool                    `json:"scanOnlyInUse"`
	MaxConcurrentImageScans   int                     `json:"maxConcurrentImageScans"`
	MaxConcurrentLayerReaders int                     `json:"maxConcurrentLayerReaders"`
	Delegates                 []runtimeDelegateConfig `json:"delegates"`
}

type runtimeDelegateConfig struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Endpoint     string   `json:"endpoint"`
	Priority     int      `json:"priority"`
	Namespaces   []string `json:"namespaces,omitempty"`
	Snapshotters []string `json:"snapshotters,omitempty"`
	ReadOnly     bool     `json:"readonly"`
	HostPath     string   `json:"hostPath"`
}

func DaemonSet(image, renderImage, integrationMRN, clusterUID string, m *v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig) (*appsv1.DaemonSet, error) {
	return daemonSet(image, renderImage, integrationMRN, clusterUID, m, cfg, nil)
}

func DaemonSetForScannerSet(image, renderImage, integrationMRN, clusterUID string, m *v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig, set v1alpha2.NodeScannerSet) (*appsv1.DaemonSet, error) {
	return daemonSet(image, renderImage, integrationMRN, clusterUID, m, cfg, &set)
}

func daemonSet(image, renderImage, integrationMRN, clusterUID string, m *v1alpha2.MondooAuditConfig, cfg v1alpha2.MondooOperatorConfig, set *v1alpha2.NodeScannerSet) (*appsv1.DaemonSet, error) {
	if err := Validate(m.Spec.Containers.RuntimeCache); err != nil {
		return nil, err
	}

	cache := RuntimeCacheWithDefaults(m.Spec.Containers.RuntimeCache)
	cache = runtimeCacheWithScannerSet(cache, set)
	scannerSetName := scannerSetNameValue(set)
	labels := LabelsForScannerSet(*m, scannerSetName)
	containerResources := k8s.ResourcesRequirementsWithDefaults(cache.Resources, k8s.DefaultRuntimeCacheScanningResources)
	gcLimit := gomemlimit.CalculateGoMemLimit(containerResources)

	cmd := []string{
		"cnspec", "serve",
		"--config", "/etc/opt/mondoo/mondoo.yml",
		"--inventory-file", runtimeCacheInventoryRenderedPath,
	}
	if serveTimerSupported(image, m.Spec.Scanner.Image) {
		cmd = append(cmd, "--timer", fmt.Sprintf("%d", intervalTimerMinutes(cache.IntervalTimer)))
	}
	if !cfg.Spec.SkipProxyForCnspec {
		if apiProxy := k8s.APIProxyURL(cfg); apiProxy != nil {
			cmd = append(cmd, "--api-proxy", *apiProxy)
		}
	}

	envVars := feature_flags.AllFeatureFlagsAsEnv()
	envVars = append(envVars,
		corev1.EnvVar{Name: "MONDOO_AUTO_UPDATE", Value: "false"},
		corev1.EnvVar{Name: "MONDOO_TMP_DIR", Value: "/tmp"},
		corev1.EnvVar{Name: "NODE_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
		corev1.EnvVar{Name: "GOMEMLIMIT", Value: gcLimit},
	)
	if !cfg.Spec.SkipProxyForCnspec {
		envVars = append(envVars, k8s.ProxyEnvVars(cfg)...)
	}
	envVars = k8s.MergeEnv(envVars, cache.Env)

	volumeMounts := []corev1.VolumeMount{
		{Name: configVolumeName, ReadOnly: true, MountPath: "/etc/opt/mondoo"},
		{Name: tempVolumeName, MountPath: "/tmp"},
	}
	volumes := []corev1.Volume{
		configVolume(*m),
		{
			Name: tempVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: cache.TempStorageSize},
			},
		},
	}

	delegateMounts := map[string]string{}
	for _, delegate := range sortedDelegates(cache.Delegates) {
		hostMountPath := path.Clean(delegate.HostPath)
		containerMountPath := delegateContainerHostPath(delegate.HostPath)
		if existing, ok := delegateMounts[containerMountPath]; ok {
			return nil, fmt.Errorf("containers.runtimeCache.delegates[%s].hostPath collides with another delegate at %q", delegate.Name, existing)
		}
		volumeName := DelegateVolumeName(delegate.Name)
		delegateMounts[containerMountPath] = hostMountPath
		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostMountPath,
					Type: ptr.To(corev1.HostPathSocket),
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			ReadOnly:  true,
			MountPath: containerMountPath,
		})
	}

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DaemonSetNameForScannerSet(m.Name, scannerSetName),
			Namespace: m.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: ptr.To(intstr.FromString("10%")),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName:           cache.ServiceAccountName,
					AutomountServiceAccountToken: ptr.To(true),
					NodeSelector:                 cache.NodeSelector,
					Affinity:                     cache.Affinity,
					Tolerations:                  mergeTolerations(cache.Tolerations),
					PriorityClassName:            cache.PriorityClassName,
					InitContainers: []corev1.Container{
						renderRuntimeCacheConfigInitContainer(renderImage),
					},
					Containers: []corev1.Container{
						{
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Name:            "mondoo-runtime-cache-scan",
							Command:         cmd,
							Resources:       containerResources,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								ReadOnlyRootFilesystem:   ptr.To(true),
								RunAsNonRoot:             ptr.To(false),
								RunAsUser:                ptr.To(int64(0)),
								Privileged:               ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
							VolumeMounts:             volumeMounts,
							Env:                      envVars,
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: corev1.TerminationMessageReadFile,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	if len(cfg.Spec.ImagePullSecrets) > 0 {
		ds.Spec.Template.Spec.ImagePullSecrets = append(ds.Spec.Template.Spec.ImagePullSecrets, cfg.Spec.ImagePullSecrets...)
	}

	return ds, nil
}

func ConfigMap(integrationMRN, clusterUID string, m v1alpha2.MondooAuditConfig) (*corev1.ConfigMap, error) {
	inv, err := Inventory(integrationMRN, clusterUID, m)
	if err != nil {
		return nil, err
	}
	delegates, err := DelegateConfig(m.Spec.Containers.RuntimeCache)
	if err != nil {
		return nil, err
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName(m.Name),
			Namespace: m.Namespace,
			Labels:    Labels(m),
		},
		Data: map[string]string{
			"inventory": inv,
			"delegates": delegates,
		},
	}, nil
}

func Inventory(integrationMRN, clusterUID string, m v1alpha2.MondooAuditConfig) (string, error) {
	cache := RuntimeCacheWithDefaults(m.Spec.Containers.RuntimeCache)
	inv := &inventory.Inventory{
		Metadata: &inventory.ObjectMeta{
			Name: "mondoo-runtime-cache-inventory",
		},
		Spec: &inventory.InventorySpec{
			Assets: []*inventory.Asset{
				{
					Id:   "runtime-cache",
					Name: runtimeCacheNodeNamePlaceholder + "-runtime-cache",
					Connections: []*inventory.Config{
						{
							Type: "k8s",
							Options: map[string]string{
								"runtime-cache":                         "true",
								"runtime-cache-node-name":               runtimeCacheNodeNamePlaceholder,
								"runtime-cache-delegates-file":          runtimeCacheDelegatesRenderedPath,
								"runtime-cache-allow-pull":              fmt.Sprintf("%t", cache.AllowPull),
								"runtime-cache-scan-only-in-use":        fmt.Sprintf("%t", derefBool(cache.ScanOnlyInUse, true)),
								"runtime-cache-max-concurrent-images":   fmt.Sprintf("%d", cache.MaxConcurrentImageScans),
								"runtime-cache-max-concurrent-layer-io": fmt.Sprintf("%d", cache.MaxConcurrentLayerReaders),
								"runtime-cache-scanner-pod-selector":    runtimeCacheScannerPodSelector(m),
								"disable-cache":                         "false",
							},
							Discover: &inventory.Discovery{Targets: []string{"runtime-cache-images"}},
						},
					},
					Labels: map[string]string{
						"k8s.mondoo.com/kind": "node",
						"mondoo.com/scan":     "runtime-cache",
					},
					ManagedBy: mondoo.ManagedByLabel(clusterUID),
				},
			},
		},
	}

	if integrationMRN != "" {
		for i := range inv.Spec.Assets {
			inv.Spec.Assets[i].Labels[constants.MondooAssetsIntegrationLabel] = integrationMRN
		}
	}
	if len(m.Spec.Annotations) > 0 {
		for i := range inv.Spec.Assets {
			inv.Spec.Assets[i].AddAnnotations(m.Spec.Annotations)
		}
	}
	for i := range inv.Spec.Assets {
		inv.Spec.Assets[i].AddAnnotations(constants.AuditConfigAnnotations(m.Name, m.Namespace))
	}

	invBytes, err := yaml.Marshal(inv)
	if err != nil {
		return "", err
	}
	return string(invBytes), nil
}

func DelegateConfig(cache v1alpha2.RuntimeCacheScanner) (string, error) {
	if err := Validate(cache); err != nil {
		return "", err
	}
	cache = RuntimeCacheWithDefaults(cache)

	delegates := make([]runtimeDelegateConfig, 0, len(cache.Delegates))
	for _, delegate := range sortedDelegates(cache.Delegates) {
		delegates = append(delegates, runtimeDelegateConfig{
			ID:           delegate.Name,
			Kind:         string(delegate.Kind),
			Endpoint:     delegateEndpoint(delegate),
			Priority:     delegate.Priority,
			Namespaces:   sortedStrings(delegate.Namespaces),
			Snapshotters: sortedStrings(delegate.Snapshotters),
			ReadOnly:     derefBool(delegate.ReadOnly, true),
			HostPath:     delegate.HostPath,
		})
	}

	out, err := yaml.Marshal(delegateConfigFile{
		RuntimeImageCache: runtimeImageCacheConfig{
			NodeName:                  runtimeCacheNodeNamePlaceholder,
			AllowPull:                 cache.AllowPull,
			ScanOnlyInUse:             derefBool(cache.ScanOnlyInUse, true),
			MaxConcurrentImageScans:   cache.MaxConcurrentImageScans,
			MaxConcurrentLayerReaders: cache.MaxConcurrentLayerReaders,
			Delegates:                 delegates,
		},
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func renderRuntimeCacheConfigInitContainer(image string) corev1.Container {
	render := fmt.Sprintf(
		"mkdir -p %s && sed \"s#%s#${NODE_NAME}#g\" %s > %s && sed \"s#%s#${NODE_NAME}#g\" %s > %s",
		shellQuote(path.Dir(runtimeCacheDelegatesRenderedPath)),
		runtimeCacheNodeNamePlaceholder,
		shellQuote(runtimeCacheInventoryTemplatePath),
		shellQuote(runtimeCacheInventoryRenderedPath),
		runtimeCacheNodeNamePlaceholder,
		shellQuote(runtimeCacheDelegatesTemplatePath),
		shellQuote(runtimeCacheDelegatesRenderedPath),
	)

	return corev1.Container{
		Name:            "render-runtime-cache-config",
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/bin/sh", "-ec", render},
		Env: []corev1.EnvVar{
			{Name: "NODE_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("16Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			ReadOnlyRootFilesystem:   ptr.To(true),
			RunAsNonRoot:             ptr.To(true),
			RunAsUser:                ptr.To(int64(65532)),
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: configVolumeName, ReadOnly: true, MountPath: "/etc/opt/mondoo"},
			{Name: tempVolumeName, MountPath: "/tmp"},
		},
	}
}

func shellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

func RuntimeCacheWithDefaults(cache v1alpha2.RuntimeCacheScanner) v1alpha2.RuntimeCacheScanner {
	if cache.Mode == "" {
		cache.Mode = v1alpha2.RuntimeCacheScannerMode_DaemonSet
	}
	if cache.ServiceAccountName == "" {
		cache.ServiceAccountName = DefaultServiceAccount
	}
	if cache.ScanOnlyInUse == nil {
		cache.ScanOnlyInUse = ptr.To(true)
	}
	if cache.IntervalTimer == 0 {
		cache.IntervalTimer = defaultIntervalTimerSeconds
	}
	if cache.TempStorageSize == nil {
		cache.TempStorageSize = defaultTempStorageSize()
	}
	if cache.MaxConcurrentImageScans == 0 {
		cache.MaxConcurrentImageScans = 1
	}
	if cache.MaxConcurrentLayerReaders == 0 {
		cache.MaxConcurrentLayerReaders = 2
	}
	return cache
}

func defaultTempStorageSize() *resource.Quantity {
	quantity := resource.MustParse("1Gi")
	return &quantity
}

func intervalTimerMinutes(seconds int) int {
	if seconds <= 0 {
		seconds = defaultIntervalTimerSeconds
	}
	return (seconds + 59) / 60
}

func serveTimerSupported(image string, scannerImage v1alpha2.Image) bool {
	if scannerImage.Digest != "" {
		return true
	}
	if scannerImage.Tag != "" {
		return scannerImage.Tag != "latest"
	}
	return !strings.HasSuffix(image, ":latest")
}

func runtimeCacheWithScannerSet(cache v1alpha2.RuntimeCacheScanner, set *v1alpha2.NodeScannerSet) v1alpha2.RuntimeCacheScanner {
	if set == nil {
		return cache
	}
	if resourceRequirementsConfigured(set.Resources) {
		cache.Resources = set.Resources
	}
	if len(set.NodeSelector) > 0 {
		cache.NodeSelector = set.NodeSelector
	}
	if set.Affinity != nil {
		cache.Affinity = set.Affinity
	}
	if len(set.Tolerations) > 0 {
		cache.Tolerations = set.Tolerations
	}
	if set.PriorityClassName != "" {
		cache.PriorityClassName = set.PriorityClassName
	}
	if len(set.Env) > 0 {
		cache.Env = append(cache.Env, set.Env...)
	}
	return cache
}

func resourceRequirementsConfigured(resources corev1.ResourceRequirements) bool {
	return len(resources.Limits) > 0 || len(resources.Requests) > 0
}

func Validate(cache v1alpha2.RuntimeCacheScanner) error {
	cache = RuntimeCacheWithDefaults(cache)
	// Runtime-cache scanning intentionally starts with no-pull, read-only containerd support only.
	// Relaxing these policy checks requires matching provider and scanner capability changes.
	if cache.Mode != v1alpha2.RuntimeCacheScannerMode_DaemonSet {
		return fmt.Errorf("containers.runtimeCache.mode %q is not supported", cache.Mode)
	}
	if cache.AllowPull {
		return fmt.Errorf("containers.runtimeCache.allowPull must be false")
	}
	if cache.MaxConcurrentImageScans < 1 {
		return fmt.Errorf("containers.runtimeCache.maxConcurrentImageScans must be at least 1")
	}
	if cache.MaxConcurrentLayerReaders < 1 {
		return fmt.Errorf("containers.runtimeCache.maxConcurrentLayerReaders must be at least 1")
	}
	if len(cache.Delegates) == 0 {
		return fmt.Errorf("containers.runtimeCache.delegates must contain at least one delegate")
	}
	if err := validateScannerSets("containers.runtimeCache.scannerSets", cache.ScannerSets); err != nil {
		return err
	}

	seen := map[string]struct{}{}
	seenHostPaths := map[string]string{}
	for _, delegate := range cache.Delegates {
		if errs := k8svalidation.IsDNS1123Label(delegate.Name); len(errs) > 0 {
			return fmt.Errorf("containers.runtimeCache.delegates[%s].name must be a valid DNS label: %s", delegate.Name, strings.Join(errs, "; "))
		}
		if _, ok := seen[delegate.Name]; ok {
			return fmt.Errorf("containers.runtimeCache.delegates contains duplicate name %q", delegate.Name)
		}
		seen[delegate.Name] = struct{}{}
		switch delegate.Kind {
		case v1alpha2.RuntimeCacheDelegateKind_Containerd:
		case v1alpha2.RuntimeCacheDelegateKind_CRI,
			v1alpha2.RuntimeCacheDelegateKind_CRIO,
			v1alpha2.RuntimeCacheDelegateKind_Docker,
			v1alpha2.RuntimeCacheDelegateKind_Podman:
			return fmt.Errorf("containers.runtimeCache.delegates[%s].kind %q is reserved for future support; only %q is supported", delegate.Name, delegate.Kind, v1alpha2.RuntimeCacheDelegateKind_Containerd)
		default:
			return fmt.Errorf("containers.runtimeCache.delegates[%s].kind %q is not supported", delegate.Name, delegate.Kind)
		}
		if !strings.HasPrefix(delegate.HostPath, "/") {
			return fmt.Errorf("containers.runtimeCache.delegates[%s].hostPath must be absolute", delegate.Name)
		}
		cleanHostPath := path.Clean(delegate.HostPath)
		if existing, ok := seenHostPaths[cleanHostPath]; ok {
			return fmt.Errorf("containers.runtimeCache.delegates[%s].hostPath duplicates delegate %q after path cleaning", delegate.Name, existing)
		}
		seenHostPaths[cleanHostPath] = delegate.Name
		if delegate.ReadOnly != nil && !*delegate.ReadOnly {
			return fmt.Errorf("containers.runtimeCache.delegates[%s].readOnly must be true", delegate.Name)
		}
	}
	return nil
}

func validateScannerSets(path string, sets []v1alpha2.NodeScannerSet) error {
	seen := map[string]struct{}{}
	for i, set := range sets {
		if set.Name == "" {
			return fmt.Errorf("%s[%d].name is required", path, i)
		}
		if errs := k8svalidation.IsDNS1123Label(set.Name); len(errs) > 0 {
			return fmt.Errorf("%s[%s].name must be a valid DNS label: %s", path, set.Name, strings.Join(errs, "; "))
		}
		if _, ok := seen[set.Name]; ok {
			return fmt.Errorf("%s[%s].name must be unique", path, set.Name)
		}
		seen[set.Name] = struct{}{}
	}
	return nil
}

func DaemonSetName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, DaemonSetNameSuffix)
}

func DaemonSetNameForScannerSet(prefix, scannerSetName string) string {
	if scannerSetName == "" {
		return DaemonSetName(prefix)
	}
	base := fmt.Sprintf("%s-runtime-cache-", prefix)
	return fmt.Sprintf("%s%s", base, scannerSetNameOrHash(k8s.ResourceNameMaxLength-len(base), scannerSetName))
}

func ConfigMapName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, InventoryConfigMapSuffix)
}

func Labels(m v1alpha2.MondooAuditConfig) map[string]string {
	return map[string]string{
		"app":       "mondoo-runtime-cache-scan",
		"scan":      "runtime-cache",
		"mondoo_cr": m.Name,
	}
}

func runtimeCacheScannerPodSelector(m v1alpha2.MondooAuditConfig) string {
	labels := Labels(m)
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, labels[key]))
	}
	return strings.Join(parts, ",")
}

func LabelsForScannerSet(m v1alpha2.MondooAuditConfig, scannerSetName string) map[string]string {
	labels := Labels(m)
	if scannerSetName != "" {
		labels["scanner_set"] = scannerSetName
	}
	return labels
}

func scannerSetNameValue(set *v1alpha2.NodeScannerSet) string {
	if set == nil {
		return ""
	}
	return strings.TrimSpace(set.Name)
}

func scannerSetNameOrHash(allowedLen int, name string) string {
	name = sanitizeDNSLabel(name)
	if allowedLen <= 0 {
		sum := fmt.Sprintf("%x", sha256.Sum256([]byte(name)))
		return sum[:8]
	}
	if len(name) <= allowedLen {
		return name
	}
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(name)))
	if allowedLen <= 8 {
		return sum[:allowedLen]
	}
	return fmt.Sprintf("%s-%s", strings.TrimRight(name[:allowedLen-9], "-"), sum[:8])
}

func DelegateVolumeName(name string) string {
	sanitized := sanitizeDNSLabel(name)
	base := "runtime-" + sanitized
	if len(base) <= 63 {
		return base
	}
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(base)))
	return fmt.Sprintf("%s-%s", strings.TrimSuffix(base[:54], "-"), sum[:8])
}

func delegateEndpoint(delegate v1alpha2.RuntimeCacheDelegate) string {
	if delegate.HostPath != "" {
		return "unix://" + delegateContainerHostPath(delegate.HostPath)
	}
	return delegate.Endpoint
}

func delegateContainerHostPath(hostPath string) string {
	return path.Join("/host", path.Clean(hostPath))
}

func configVolume(m v1alpha2.MondooAuditConfig) corev1.Volume {
	return corev1.Volume{
		Name: configVolumeName,
		VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				DefaultMode: ptr.To(int32(corev1.ProjectedVolumeSourceDefaultMode)),
				Sources: []corev1.VolumeProjection{
					{
						ConfigMap: &corev1.ConfigMapProjection{
							LocalObjectReference: corev1.LocalObjectReference{Name: ConfigMapName(m.Name)},
							Items: []corev1.KeyToPath{
								{Key: "inventory", Path: "runtime-cache-inventory.yml"},
								{Key: "delegates", Path: "runtime-cache/delegates.yml"},
							},
						},
					},
					{
						Secret: &corev1.SecretProjection{
							LocalObjectReference: k8s.ConfigSecretRef(m),
							Items:                []corev1.KeyToPath{{Key: "config", Path: "mondoo.yml"}},
						},
					},
				},
			},
		},
	}
}

func sortedDelegates(in []v1alpha2.RuntimeCacheDelegate) []v1alpha2.RuntimeCacheDelegate {
	out := append([]v1alpha2.RuntimeCacheDelegate{}, in...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority == out[j].Priority {
			return out[i].Name < out[j].Name
		}
		return out[i].Priority < out[j].Priority
	})
	return out
}

func sortedStrings(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

func mergeTolerations(groups ...[]corev1.Toleration) []corev1.Toleration {
	seen := map[string]corev1.Toleration{}
	for _, group := range groups {
		for _, toleration := range group {
			seen[tolerationKey(toleration)] = toleration
		}
	}

	out := make([]corev1.Toleration, 0, len(seen))
	for _, toleration := range seen {
		out = append(out, toleration)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return tolerationKey(out[i]) < tolerationKey(out[j])
	})
	return out
}

func tolerationKey(t corev1.Toleration) string {
	tolerationSeconds := ""
	if t.TolerationSeconds != nil {
		tolerationSeconds = fmt.Sprintf("%d", *t.TolerationSeconds)
	}
	return strings.Join([]string{t.Key, string(t.Operator), t.Value, string(t.Effect), tolerationSeconds}, "\x00")
}

func derefBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func sanitizeDNSLabel(in string) string {
	in = strings.ToLower(in)
	var b strings.Builder
	lastDash := false
	for _, r := range in {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "delegate"
	}
	return out
}
