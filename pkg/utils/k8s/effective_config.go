// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
)

func EffectiveSpec(spec v1alpha2.MondooAuditConfigSpec, remoteConfig string, uid types.UID) (v1alpha2.MondooAuditConfigSpec, []string, error) {
	if !spec.RemoteManaged || remoteConfig == "" {
		return spec, nil, nil
	}

	var cfg mondooclient.K8sIntegrationConfig
	if err := json.Unmarshal([]byte(remoteConfig), &cfg); err != nil {
		return v1alpha2.MondooAuditConfigSpec{}, nil, fmt.Errorf("parsing remote config: %w", err)
	}

	var warnings []string
	effective := mapRemoteConfig(&cfg, uid, &warnings)
	preserveLocalFields(&effective, &spec)
	applyDefaults(&effective)

	return effective, warnings, nil
}

func mapRemoteConfig(cfg *mondooclient.K8sIntegrationConfig, uid types.UID, warnings *[]string) v1alpha2.MondooAuditConfigSpec {
	spec := v1alpha2.MondooAuditConfigSpec{}

	spec.KubernetesResources.Enable = cfg.ScanWorkloads
	spec.Nodes.Enable = cfg.ScanNodes
	spec.Containers.Enable = cfg.ScanPublicImages

	if !cfg.ScanLocalCluster {
		spec.KubernetesResources.Enable = false
		spec.Nodes.Enable = false
	}

	style := strings.ToLower(cfg.ScanNodesStyle)
	switch style {
	case "cronjob", "deployment", "daemonset":
		spec.Nodes.Style = v1alpha2.NodeScanStyle(style)
	default:
		spec.Nodes.Style = v1alpha2.NodeScanStyle_CronJob
	}

	spec.Filtering.Namespaces.Include = cfg.NamespaceAllowList
	spec.Filtering.Namespaces.Exclude = cfg.NamespaceDenyList

	spec.KubernetesResources.Schedule = scheduleOrDefault(cfg.Schedule, uid, 0)
	spec.Nodes.Schedule = scheduleOrDefault(cfg.NodesSchedule, uid, 1)
	spec.Containers.Schedule = scheduleOrDefault(cfg.ContainersSchedule, uid, 2)

	replicas := cfg.ScannerReplicas
	spec.Scanner.Replicas = &replicas

	spec.Scanner.Resources = mapResources(cfg.ScannerResources, "scannerResources", warnings)
	spec.Nodes.Resources = mapResources(cfg.NodesResources, "nodesResources", warnings)
	spec.Containers.Resources = mapResources(cfg.ContainersResources, "containersResources", warnings)

	if cfg.ResourceWatcher != nil {
		spec.KubernetesResources.ResourceWatcher = v1alpha2.ResourceWatcherSpec{
			Enable:              cfg.ResourceWatcher.Enable,
			DebounceInterval:    parseDuration(cfg.ResourceWatcher.DebounceInterval),
			MinimumScanInterval: parseDuration(cfg.ResourceWatcher.MinimumScanInterval),
			WatchAllResources:   cfg.ResourceWatcher.WatchAllResources,
			ResourceTypes:       cfg.ResourceWatcher.ResourceTypes,
		}
	}

	spec.Containers.Repositories.Include = cfg.ContainerRepositoriesAllowList
	spec.Containers.Repositories.Exclude = cfg.ContainerRepositoriesDenyList

	if cfg.ScanCacheEnabled {
		scanCache := &v1alpha2.ScanCacheConfig{Enable: true}
		if cfg.ScanCacheTTL != "" {
			if d, err := time.ParseDuration(cfg.ScanCacheTTL); err == nil {
				dur := metav1.Duration{Duration: d}
				scanCache.CacheTTL = &dur
			}
		}
		spec.Containers.ScanCache = scanCache
	}

	if cfg.K8sActiveDeadline > 0 {
		d := metav1.Duration{Duration: time.Duration(cfg.K8sActiveDeadline) * time.Second}
		spec.KubernetesResources.ActiveDeadline = &d
	}
	if cfg.ContainersActiveDeadline > 0 {
		d := metav1.Duration{Duration: time.Duration(cfg.ContainersActiveDeadline) * time.Second}
		spec.Containers.ActiveDeadline = &d
	}

	spec.JobOverrides = mapJobOverrides(cfg.JobOverrides)
	spec.KubernetesResources.JobOverrides = mapJobOverrides(cfg.ScannerJobOverrides)
	spec.Nodes.JobOverrides = mapJobOverrides(cfg.NodesJobOverrides)
	spec.Containers.JobOverrides = mapJobOverrides(cfg.ContainersJobOverrides)

	spec.Annotations = cfg.AssetAnnotations
	spec.SpaceID = cfg.SpaceID

	spec.Nodes.PriorityClassName = cfg.NodesPriorityClassName
	spec.Nodes.IntervalTimer = int(cfg.NodesIntervalTimer)

	spec.Scanner.Env = mapEnvVars(cfg.ScannerEnv)
	spec.Nodes.Env = mapEnvVars(cfg.NodesEnv)
	spec.Containers.Env = mapEnvVars(cfg.ContainersEnv)

	spec.KubernetesResources.ExternalClusters = mapExternalClusters(cfg.ExternalClusters)

	spec.InheritNamespaceLabels = cfg.InheritNamespaceLabels
	spec.Filtering.NamespaceLabelSelector = mapLabelSelector(cfg.NamespaceLabelSelector)
	spec.Filtering.ObjectLabelSelector = mapLabelSelector(cfg.ObjectLabelSelector)
	spec.KubernetesResources.NetworkInventory = mapNetworkInventory(cfg.NetworkInventory)

	spec.KubernetesResources.Suspend = cfg.K8sSuspend
	spec.Nodes.Suspend = cfg.NodesSuspend
	spec.Containers.Suspend = cfg.ContainersSuspend

	spec.Nodes.Profiles = mapNodeProfiles(cfg.NodesProfiles, warnings)
	spec.Nodes.ScheduleSpread = parseDuration(cfg.NodesScheduleSpread)
	spec.Nodes.LabelSelector = mapLabelSelector(cfg.NodesLabelSelector)

	for _, name := range cfg.PrivateRegistriesPullSecretRefs {
		spec.Scanner.PrivateRegistriesPullSecretRefs = append(
			spec.Scanner.PrivateRegistriesPullSecretRefs,
			corev1.LocalObjectReference{Name: name},
		)
	}

	if cfg.ContainersWif != nil {
		spec.Containers.WorkloadIdentity = mapContainersWif(cfg.ContainersWif)
	}

	return spec
}

func preserveLocalFields(effective, local *v1alpha2.MondooAuditConfigSpec) {
	effective.MondooCredsSecretRef = local.MondooCredsSecretRef
	effective.MondooTokenSecretRef = local.MondooTokenSecretRef
	effective.ConsoleIntegration = local.ConsoleIntegration
	effective.RemoteManaged = local.RemoteManaged
	effective.Scanner.ServiceAccountName = local.Scanner.ServiceAccountName
	effective.Scanner.Image = local.Scanner.Image
	effective.Scanner.InitContainerImagePullPolicy = local.Scanner.InitContainerImagePullPolicy
}

func applyDefaults(spec *v1alpha2.MondooAuditConfigSpec) {
	if spec.Scanner.Replicas != nil && *spec.Scanner.Replicas == 0 {
		one := int32(1)
		spec.Scanner.Replicas = &one
	}
	if spec.Nodes.Enable && spec.Nodes.Style == "" {
		spec.Nodes.Style = v1alpha2.NodeScanStyle_CronJob
	}
}

func scheduleOrDefault(schedule string, uid types.UID, index int) string {
	if schedule != "" {
		return schedule
	}
	h := sha256.Sum256([]byte(uid))
	minute := int(h[index]) % 60
	return fmt.Sprintf("%d * * * *", minute)
}

func mapResources(cfg *mondooclient.K8sResourceRequirementsConfig, field string, warnings *[]string) corev1.ResourceRequirements {
	if cfg == nil {
		return corev1.ResourceRequirements{}
	}
	reqs := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}
	parseField := func(val, subfield string, list corev1.ResourceList, key corev1.ResourceName) {
		if val == "" {
			return
		}
		q, err := resource.ParseQuantity(val)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("invalid %s.%s %q: %v", field, subfield, val, err))
			return
		}
		list[key] = q
	}
	parseField(cfg.CPURequest, "cpuRequest", reqs.Requests, corev1.ResourceCPU)
	parseField(cfg.CPULimit, "cpuLimit", reqs.Limits, corev1.ResourceCPU)
	parseField(cfg.MemRequest, "memRequest", reqs.Requests, corev1.ResourceMemory)
	parseField(cfg.MemLimit, "memLimit", reqs.Limits, corev1.ResourceMemory)
	return reqs
}

func parseDuration(s string) metav1.Duration {
	if s == "" {
		return metav1.Duration{}
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return metav1.Duration{}
	}
	return metav1.Duration{Duration: d}
}

func mapJobOverrides(cfg *mondooclient.K8sJobOverridesConfig) v1alpha2.JobOverrides {
	if cfg == nil {
		return v1alpha2.JobOverrides{}
	}
	jo := v1alpha2.JobOverrides{
		Annotations:  cfg.Annotations,
		NodeSelector: cfg.NodeSelector,
		Labels:       cfg.Labels,
	}
	if cfg.TTLSecondsAfterFinished != 0 {
		ttl := cfg.TTLSecondsAfterFinished
		jo.TTLSecondsAfterFinished = &ttl
	}
	jo.Tolerations = mapTolerations(cfg.Tolerations)
	return jo
}

func mapTolerations(cfgs []mondooclient.K8sTolerationConfig) []corev1.Toleration {
	var tolerations []corev1.Toleration
	for _, t := range cfgs {
		toleration := corev1.Toleration{
			Key:      t.Key,
			Operator: corev1.TolerationOperator(t.Operator),
			Value:    t.Value,
			Effect:   corev1.TaintEffect(t.Effect),
		}
		if t.TolerationSeconds != nil {
			secs := int64(*t.TolerationSeconds)
			toleration.TolerationSeconds = &secs
		}
		tolerations = append(tolerations, toleration)
	}
	return tolerations
}

func mapLabelSelector(cfg *mondooclient.K8sLabelSelectorConfig) *metav1.LabelSelector {
	if cfg == nil || (len(cfg.MatchLabels) == 0 && len(cfg.MatchExpressions) == 0) {
		return nil
	}
	sel := &metav1.LabelSelector{MatchLabels: cfg.MatchLabels}
	for _, e := range cfg.MatchExpressions {
		sel.MatchExpressions = append(sel.MatchExpressions, metav1.LabelSelectorRequirement{
			Key:      e.Key,
			Operator: metav1.LabelSelectorOperator(e.Operator),
			Values:   e.Values,
		})
	}
	return sel
}

func mapNetworkInventory(cfg *mondooclient.K8sNetworkInventoryConfig) v1alpha2.NetworkInventorySpec {
	if cfg == nil {
		return v1alpha2.NetworkInventorySpec{}
	}
	return v1alpha2.NetworkInventorySpec{
		HBN: v1alpha2.HBNNetworkInventorySpec{
			Enable:                 cfg.HbnEnable,
			IncludeLegacyResources: cfg.HbnIncludeLegacyResources,
		},
		MultiNetworkPolicy: v1alpha2.MultiNetworkPolicyInventorySpec{
			Enable: cfg.MultiNetworkPolicyEnable,
		},
		Classifications: v1alpha2.NetworkInventoryClassifications{
			PublicCIDRs:        cfg.PublicCidrs,
			PrivateCIDRs:       cfg.PrivateCidrs,
			TrustedEgressCIDRs: cfg.TrustedEgressCidrs,
		},
	}
}

func mapNodeProfiles(cfgs []mondooclient.K8sNodeScanProfileConfig, warnings *[]string) []v1alpha2.NodeScanProfile {
	if len(cfgs) == 0 {
		return nil
	}
	profiles := make([]v1alpha2.NodeScanProfile, 0, len(cfgs))
	for i, p := range cfgs {
		profiles = append(profiles, v1alpha2.NodeScanProfile{
			Name:         p.Name,
			NodeSelector: p.NodeSelector,
			Resources:    mapResources(p.Resources, fmt.Sprintf("nodesProfiles[%d].resources", i), warnings),
			Tolerations:  mapTolerations(p.Tolerations),
		})
	}
	return profiles
}

func mapEnvVars(cfgEnvs []mondooclient.K8sEnvVarConfig) []corev1.EnvVar {
	if len(cfgEnvs) == 0 {
		return nil
	}
	envs := make([]corev1.EnvVar, 0, len(cfgEnvs))
	for _, e := range cfgEnvs {
		envs = append(envs, corev1.EnvVar{Name: e.Name, Value: e.Value, ValueFrom: mapEnvVarSource(e.ValueFrom)})
	}
	return envs
}

func mapEnvVarSource(cfg *mondooclient.K8sEnvVarSourceConfig) *corev1.EnvVarSource {
	if cfg == nil {
		return nil
	}
	src := &corev1.EnvVarSource{}
	if cfg.SecretKeyRef != nil {
		src.SecretKeyRef = &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: cfg.SecretKeyRef.Name},
			Key:                  cfg.SecretKeyRef.Key,
			Optional:             optionalBool(cfg.SecretKeyRef.Optional),
		}
	}
	if cfg.ConfigMapKeyRef != nil {
		src.ConfigMapKeyRef = &corev1.ConfigMapKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: cfg.ConfigMapKeyRef.Name},
			Key:                  cfg.ConfigMapKeyRef.Key,
			Optional:             optionalBool(cfg.ConfigMapKeyRef.Optional),
		}
	}
	if cfg.FieldPath != "" {
		src.FieldRef = &corev1.ObjectFieldSelector{FieldPath: cfg.FieldPath}
	}
	if src.SecretKeyRef == nil && src.ConfigMapKeyRef == nil && src.FieldRef == nil {
		return nil
	}
	return src
}

func optionalBool(b bool) *bool {
	if !b {
		return nil
	}
	return &b
}

func mapExternalClusters(cfgs []mondooclient.K8sExternalClusterConfig) []v1alpha2.ExternalCluster {
	if len(cfgs) == 0 {
		return nil
	}
	clusters := make([]v1alpha2.ExternalCluster, 0, len(cfgs))
	for _, c := range cfgs {
		cluster := v1alpha2.ExternalCluster{
			Name:                   c.Name,
			ContainerImageScanning: c.ContainerImageScanning,
			Schedule:               c.Schedule,
			Suspend:                c.Suspend,
		}
		if len(c.RepositoriesAllowList) > 0 || len(c.RepositoriesDenyList) > 0 {
			cluster.Repositories = &v1alpha2.FilteringSpec{
				Include: c.RepositoriesAllowList,
				Exclude: c.RepositoriesDenyList,
			}
		}
		if c.PrivateRegistriesPullSecret != "" {
			cluster.PrivateRegistriesPullSecretRef = &corev1.LocalObjectReference{Name: c.PrivateRegistriesPullSecret}
		}
		if c.KubeconfigSecret != "" {
			ref := corev1.LocalObjectReference{Name: c.KubeconfigSecret}
			cluster.KubeconfigSecretRef = &ref
		}
		if c.Server != "" {
			sa := &v1alpha2.ServiceAccountAuth{
				Server:        c.Server,
				SkipTLSVerify: c.SkipTlsVerify,
			}
			if c.CredentialsSecret != "" {
				sa.CredentialsSecretRef = corev1.LocalObjectReference{Name: c.CredentialsSecret}
			}
			cluster.ServiceAccountAuth = sa
		}
		filtering := v1alpha2.Filtering{
			Namespaces: v1alpha2.FilteringSpec{
				Include: c.NamespaceAllowList,
				Exclude: c.NamespaceDenyList,
			},
			NamespaceLabelSelector: mapLabelSelector(c.NamespaceLabelSelector),
			ObjectLabelSelector:    mapLabelSelector(c.ObjectLabelSelector),
		}
		if len(filtering.Namespaces.Include) > 0 || len(filtering.Namespaces.Exclude) > 0 ||
			filtering.NamespaceLabelSelector != nil || filtering.ObjectLabelSelector != nil {
			cluster.Filtering = &filtering
		}
		if c.WifProvider != "" {
			wif := mapExternalClusterWif(c)
			cluster.WorkloadIdentity = wif
		}
		if c.Spiffe != nil {
			cluster.SPIFFEAuth = &v1alpha2.SPIFFEAuthConfig{
				Server:     c.Server,
				SocketPath: c.Spiffe.SocketPath,
				Audience:   c.Spiffe.Audience,
			}
			if c.Spiffe.TrustBundleSecret != "" {
				cluster.SPIFFEAuth.TrustBundleSecretRef = corev1.LocalObjectReference{Name: c.Spiffe.TrustBundleSecret}
			}
		}
		if c.Vault != nil {
			cluster.VaultAuth = &v1alpha2.VaultAuthConfig{
				Server:              c.Server,
				VaultAddr:           c.Vault.VaultAddr,
				VaultNamespace:      c.Vault.VaultNamespace,
				AuthRole:            c.Vault.AuthRole,
				CredsRole:           c.Vault.CredsRole,
				AuthPath:            c.Vault.AuthPath,
				SecretsPath:         c.Vault.SecretsPath,
				KubernetesNamespace: c.Vault.KubernetesNamespace,
				TTL:                 c.Vault.TTL,
			}
			if c.Vault.CaCertSecret != "" {
				ref := corev1.LocalObjectReference{Name: c.Vault.CaCertSecret}
				cluster.VaultAuth.CACertSecretRef = &ref
			}
			if c.Vault.TargetCaCertSecret != "" {
				ref := corev1.LocalObjectReference{Name: c.Vault.TargetCaCertSecret}
				cluster.VaultAuth.TargetCACertSecretRef = &ref
			}
		}
		clusters = append(clusters, cluster)
	}
	return clusters
}

func mapExternalClusterWif(c mondooclient.K8sExternalClusterConfig) *v1alpha2.WorkloadIdentityConfig {
	wif := &v1alpha2.WorkloadIdentityConfig{
		Provider: v1alpha2.CloudProvider(strings.ToLower(c.WifProvider)),
	}
	if c.Gke != nil {
		wif.GKE = &v1alpha2.GKEWorkloadIdentity{
			ProjectID:            c.Gke.ProjectID,
			ClusterName:          c.Gke.ClusterName,
			ClusterLocation:      c.Gke.ClusterLocation,
			GoogleServiceAccount: c.Gke.GoogleServiceAccount,
			Endpoint:             c.Gke.Endpoint,
		}
	}
	if c.Eks != nil {
		wif.EKS = &v1alpha2.EKSWorkloadIdentity{
			Region:      c.Eks.Region,
			ClusterName: c.Eks.ClusterName,
			RoleARN:     c.Eks.RoleARN,
			Endpoint:    c.Eks.Endpoint,
		}
	}
	if c.Aks != nil {
		wif.AKS = &v1alpha2.AKSWorkloadIdentity{
			SubscriptionID: c.Aks.SubscriptionID,
			ResourceGroup:  c.Aks.ResourceGroup,
			ClusterName:    c.Aks.ClusterName,
			ClientID:       c.Aks.ClientID,
			TenantID:       c.Aks.TenantID,
			Endpoint:       c.Aks.Endpoint,
			LoginServer:    c.Aks.LoginServer,
		}
	}
	return wif
}

func mapContainersWif(cfg *mondooclient.K8sContainersWifConfig) *v1alpha2.WorkloadIdentityConfig {
	if cfg == nil {
		return nil
	}
	wif := &v1alpha2.WorkloadIdentityConfig{
		Provider: v1alpha2.CloudProvider(strings.ToLower(cfg.Provider)),
	}
	if cfg.Gke != nil {
		wif.GKE = &v1alpha2.GKEWorkloadIdentity{
			ProjectID:            cfg.Gke.ProjectID,
			ClusterName:          cfg.Gke.ClusterName,
			ClusterLocation:      cfg.Gke.ClusterLocation,
			GoogleServiceAccount: cfg.Gke.GoogleServiceAccount,
			Endpoint:             cfg.Gke.Endpoint,
		}
	}
	if cfg.Eks != nil {
		wif.EKS = &v1alpha2.EKSWorkloadIdentity{
			Region:      cfg.Eks.Region,
			ClusterName: cfg.Eks.ClusterName,
			RoleARN:     cfg.Eks.RoleARN,
			Endpoint:    cfg.Eks.Endpoint,
		}
	}
	if cfg.Aks != nil {
		wif.AKS = &v1alpha2.AKSWorkloadIdentity{
			SubscriptionID: cfg.Aks.SubscriptionID,
			ResourceGroup:  cfg.Aks.ResourceGroup,
			ClusterName:    cfg.Aks.ClusterName,
			ClientID:       cfg.Aks.ClientID,
			TenantID:       cfg.Aks.TenantID,
			Endpoint:       cfg.Aks.Endpoint,
			LoginServer:    cfg.Aks.LoginServer,
		}
	}
	return wif
}
