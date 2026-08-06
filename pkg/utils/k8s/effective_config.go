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
	for _, t := range cfg.Tolerations {
		jo.Tolerations = append(jo.Tolerations, corev1.Toleration{
			Key:      t.Key,
			Operator: corev1.TolerationOperator(t.Operator),
			Value:    t.Value,
			Effect:   corev1.TaintEffect(t.Effect),
		})
	}
	return jo
}

func mapEnvVars(cfgEnvs []mondooclient.K8sEnvVarConfig) []corev1.EnvVar {
	if len(cfgEnvs) == 0 {
		return nil
	}
	envs := make([]corev1.EnvVar, 0, len(cfgEnvs))
	for _, e := range cfgEnvs {
		envs = append(envs, corev1.EnvVar{Name: e.Name, Value: e.Value})
	}
	return envs
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
		if len(c.NamespaceAllowList) > 0 || len(c.NamespaceDenyList) > 0 {
			cluster.Filtering = &v1alpha2.Filtering{
				Namespaces: v1alpha2.FilteringSpec{
					Include: c.NamespaceAllowList,
					Exclude: c.NamespaceDenyList,
				},
			}
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
