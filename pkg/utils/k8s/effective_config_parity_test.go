// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
)

// remoteUnmappedSpecFields are the MondooAuditConfig spec paths a remote-managed
// configuration does not set. Every other spec field must be reachable from the
// remote config, so a new field without a remote mapping fails TestRemoteConfigParity.
var remoteUnmappedSpecFields = map[string]string{ //nolint:gosec // G101: keys are spec field paths, not credentials
	"mondooCredsSecretRef":                       "local: bootstraps the connection to Mondoo",
	"mondooTokenSecretRef":                       "local: bootstraps the connection to Mondoo",
	"consoleIntegration":                         "local: bootstraps the connection to Mondoo",
	"remoteManaged":                              "local: decides whether the remote config applies at all",
	"scheduleGuard":                              "local: the minimum scan interval guard is a cluster-side safety net",
	"scanner.image":                              "local: image overrides and mirrors are per cluster",
	"scanner.serviceAccountName":                 "local: the service account exists in the cluster",
	"scanner.initContainerImagePullPolicy":       "local: tied to the local image override",
	"scanner.privateRegistriesPullSecretRef":     "deprecated: privateRegistriesPullSecretRefs replaces it",
	"kubernetesResources.containerImageScanning": "deprecated: containers.enable replaces it",
	"admission":                                  "deprecated: ignored since v12.1.0",
}

func TestRemoteConfigParity(t *testing.T) {
	var cfg mondooclient.K8sIntegrationConfig
	fillNonZero(reflect.ValueOf(&cfg).Elem())
	cfg.ScanLocalCluster = true

	raw, err := json.Marshal(cfg)
	require.NoError(t, err)

	spec, _, err := EffectiveSpec(v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}, string(raw), "uid")
	require.NoError(t, err)

	var unset []string
	collectUnset(reflect.ValueOf(spec), "", &unset)

	covered := func(path, entry string) bool {
		return path == entry || strings.HasPrefix(path, entry+".")
	}

	var missing []string
	for _, path := range unset {
		listed := false
		for entry := range remoteUnmappedSpecFields {
			listed = listed || covered(path, entry)
		}
		if !listed {
			missing = append(missing, path)
		}
	}
	assert.Empty(t, missing, "MondooAuditConfig spec fields that a remote-managed config cannot set; "+
		"map them in mapRemoteConfig or list them in remoteUnmappedSpecFields")

	for entry := range remoteUnmappedSpecFields {
		if entry == "remoteManaged" {
			// Preserved from the local spec, which must set it for the remote config to apply.
			continue
		}
		used := false
		for _, path := range unset {
			used = used || covered(path, entry)
		}
		assert.True(t, used, "%s is listed in remoteUnmappedSpecFields but the remote config sets it", entry)
	}
}

// fillNonZero sets every field reachable from v to a non-zero value. Strings get "1m",
// which parses both as a duration and as a resource quantity.
func fillNonZero(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fillNonZero(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				fillNonZero(v.Field(i))
			}
		}
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		fillNonZero(v.Index(0))
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		key := reflect.New(v.Type().Key()).Elem()
		fillNonZero(key)
		val := reflect.New(v.Type().Elem()).Elem()
		fillNonZero(val)
		m.SetMapIndex(key, val)
		v.Set(m)
	case reflect.String:
		v.SetString("1m")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	}
}

// collectUnset walks the operator's own spec types and records the json path of every
// zero-valued field. Types from other packages (corev1, metav1, resource) are leaves.
func collectUnset(v reflect.Value, path string, unset *[]string) {
	isOwnType := func(t reflect.Type) bool {
		return t.PkgPath() == reflect.TypeFor[v1alpha2.MondooAuditConfigSpec]().PkgPath()
	}

	t := v.Type()
	if v.Kind() == reflect.Pointer && isOwnType(t.Elem()) && t.Elem().Kind() == reflect.Struct {
		if v.IsNil() {
			*unset = append(*unset, path)
			return
		}
		collectUnset(v.Elem(), path, unset)
		return
	}
	if v.Kind() == reflect.Slice && isOwnType(t.Elem()) && t.Elem().Kind() == reflect.Struct {
		if v.Len() == 0 {
			*unset = append(*unset, path)
			return
		}
		collectUnset(v.Index(0), path, unset)
		return
	}
	if v.Kind() == reflect.Struct && isOwnType(t) {
		for i := 0; i < t.NumField(); i++ {
			name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
			if name == "" || name == "-" {
				continue
			}
			child := name
			if path != "" {
				child = path + "." + name
			}
			collectUnset(v.Field(i), child, unset)
		}
		return
	}
	if v.IsZero() {
		*unset = append(*unset, path)
	}
}

func TestEffectiveSpec_ParityFields(t *testing.T) {
	remote := `{
		"scanLocalCluster": true,
		"scanNodes": true,
		"inheritNamespaceLabels": ["example.com/team-id", "example.com/cost-*"],
		"namespaceLabelSelector": {"matchLabels": {"scan": "yes"}, "matchExpressions": [{"key": "tier", "operator": "In", "values": ["a", "b"]}]},
		"objectLabelSelector": {"matchExpressions": [{"key": "skip", "operator": "DoesNotExist"}]},
		"networkInventory": {"hbnEnable": false, "multiNetworkPolicyEnable": true, "publicCidrs": ["0.0.0.0/0"]},
		"k8sSuspend": true,
		"nodesSuspend": true,
		"containersSuspend": true,
		"nodesProfiles": [{"name": "gpu", "nodeSelector": {"gpu": "true"}, "resources": {"memLimit": "2Gi"}, "tolerations": [{"key": "gpu", "operator": "Exists", "effect": "NoSchedule"}]}],
		"nodesScheduleSpread": "15m",
		"nodesLabelSelector": {"matchLabels": {"pool": "default"}},
		"scannerEnv": [{"name": "TOKEN", "valueFrom": {"secretKeyRef": {"name": "s", "key": "k", "optional": true}}}, {"name": "NODE", "valueFrom": {"fieldPath": "spec.nodeName"}}],
		"scannerJobOverrides": {"tolerations": [{"key": "t", "operator": "Exists", "effect": "NoExecute", "tolerationSeconds": "60"}]},
		"externalClusters": [{
			"name": "remote",
			"kubeconfigSecret": "kc",
			"schedule": "0 * * * *",
			"suspend": true,
			"repositoriesAllowList": ["ghcr.io/acme/*"],
			"privateRegistriesPullSecret": "pull",
			"objectLabelSelector": {"matchLabels": {"app": "web"}}
		}]
	}`

	spec, warnings, err := EffectiveSpec(v1alpha2.MondooAuditConfigSpec{RemoteManaged: true}, remote, "uid")
	require.NoError(t, err)
	assert.Empty(t, warnings)

	assert.Equal(t, []string{"example.com/team-id", "example.com/cost-*"}, spec.InheritNamespaceLabels)
	assert.Equal(t, &metav1.LabelSelector{
		MatchLabels:      map[string]string{"scan": "yes"},
		MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "tier", Operator: metav1.LabelSelectorOpIn, Values: []string{"a", "b"}}},
	}, spec.Filtering.NamespaceLabelSelector)
	assert.Equal(t, &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "skip", Operator: metav1.LabelSelectorOpDoesNotExist}},
	}, spec.Filtering.ObjectLabelSelector)

	require.NotNil(t, spec.KubernetesResources.NetworkInventory.HBN.Enable)
	assert.False(t, *spec.KubernetesResources.NetworkInventory.HBN.Enable, "an explicit false must survive")
	assert.Nil(t, spec.KubernetesResources.NetworkInventory.HBN.IncludeLegacyResources, "unset keeps the default")
	require.NotNil(t, spec.KubernetesResources.NetworkInventory.MultiNetworkPolicy.Enable)
	assert.True(t, *spec.KubernetesResources.NetworkInventory.MultiNetworkPolicy.Enable)
	assert.Equal(t, []string{"0.0.0.0/0"}, spec.KubernetesResources.NetworkInventory.Classifications.PublicCIDRs)

	assert.True(t, spec.KubernetesResources.Suspend)
	assert.True(t, spec.Nodes.Suspend)
	assert.True(t, spec.Containers.Suspend)

	require.Len(t, spec.Nodes.Profiles, 1)
	assert.Equal(t, "gpu", spec.Nodes.Profiles[0].Name)
	assert.Equal(t, map[string]string{"gpu": "true"}, spec.Nodes.Profiles[0].NodeSelector)
	assert.Equal(t, "2Gi", spec.Nodes.Profiles[0].Resources.Limits.Memory().String())
	assert.Equal(t, corev1.TaintEffectNoSchedule, spec.Nodes.Profiles[0].Tolerations[0].Effect)
	assert.Equal(t, "15m0s", spec.Nodes.ScheduleSpread.Duration.String())
	assert.Equal(t, map[string]string{"pool": "default"}, spec.Nodes.LabelSelector.MatchLabels)

	require.Len(t, spec.Scanner.Env, 2)
	optional := true
	assert.Equal(t, &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "s"}, Key: "k", Optional: &optional,
	}}, spec.Scanner.Env[0].ValueFrom)
	assert.Equal(t, &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}, spec.Scanner.Env[1].ValueFrom)

	require.Len(t, spec.KubernetesResources.JobOverrides.Tolerations, 1)
	require.NotNil(t, spec.KubernetesResources.JobOverrides.Tolerations[0].TolerationSeconds)
	assert.Equal(t, int64(60), *spec.KubernetesResources.JobOverrides.Tolerations[0].TolerationSeconds)

	require.Len(t, spec.KubernetesResources.ExternalClusters, 1)
	ec := spec.KubernetesResources.ExternalClusters[0]
	assert.Equal(t, "0 * * * *", ec.Schedule)
	assert.True(t, ec.Suspend)
	assert.Equal(t, &v1alpha2.FilteringSpec{Include: []string{"ghcr.io/acme/*"}}, ec.Repositories)
	assert.Equal(t, &corev1.LocalObjectReference{Name: "pull"}, ec.PrivateRegistriesPullSecretRef)
	require.NotNil(t, ec.Filtering, "a selector alone creates the filtering block")
	assert.Equal(t, map[string]string{"app": "web"}, ec.Filtering.ObjectLabelSelector.MatchLabels)
}
