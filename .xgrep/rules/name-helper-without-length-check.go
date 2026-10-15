// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

//go:build ignore

// xgrep test fixture, not compiled. Run: xgrep test .xgrep/rules
package nodes

// --- plain concatenation: flagged ---------------------------------------------------------------

// ruleid: mondoo-name-helper-without-length-check
func DeploymentName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, DeploymentNameSuffix)
}

// ruleid: mondoo-name-helper-without-length-check
func DaemonSetName(prefix string) string {
	return prefix + DaemonSetNameBase
}

// ruleid: mondoo-name-helper-without-length-check
func ConfigMapName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, InventoryConfigMapBase)
}

// ruleid: mondoo-name-helper-without-length-check
func VaultKubeconfigSecretName(prefix, clusterName string) string {
	return fmt.Sprintf("%s-vault-kubeconfig-%s", prefix, clusterName)
}

// ruleid: mondoo-name-helper-without-length-check
func MergedSecretName(m *v1alpha2.MondooAuditConfig) string {
	return m.Name + MergedPrivateRegistriesSecretSuffix
}

// --- length-checked: not flagged ----------------------------------------------------------------

// ok: mondoo-name-helper-without-length-check
func CronJobName(prefix, suffix string) string {
	return nodeScopedName(prefix, CronJobNameBase, suffix)
}

// ok: mondoo-name-helper-without-length-check
func ExternalClusterCronJobName(prefix, clusterName string) string {
	return k8s.CronJobNameWithCluster("k8s", prefix, clusterName)
}

// ok: mondoo-name-helper-without-length-check
func ConfigMapNameWithNode(prefix, nodeName string) string {
	return nodeScopedName(prefix, InventoryConfigMapWithNodeBase, nodeName)
}

// ok: mondoo-name-helper-without-length-check
func WIFServiceAccountName(prefix, clusterName string) string {
	return k8s.CronJobNameWithCluster("wif", prefix, clusterName)
}

// ok: mondoo-name-helper-without-length-check
func hashedJobName(prefix string) string {
	h := fmt.Sprintf("%x", sha256.Sum256([]byte(prefix)))
	return "mondoo-gc-" + h[:8]
}

// the primitive itself, in package k8s: the bare constant is evidence too
// ok: mondoo-name-helper-without-length-check
func cronJobName(scanType, id, clusterName string) string {
	full := cronJobPrefix + scanType + "-" + id
	if len(full) <= ResourceNameMaxLength {
		return full
	}
	idHash := fmt.Sprintf("%x", sha256.Sum256([]byte(id)))[:hashLen]
	return full[:ResourceNameMaxLength-hashLen-1] + "-" + idHash
}

// spells the old-style name so the old object can be found and deleted: must not truncate
// ok: mondoo-name-helper-without-length-check
func OldCronJobName(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, OldCronJobNameSuffix)
}

// not a name helper: returns a set, not a string
// ok: mondoo-name-helper-without-length-check
func getPodNames(pods []corev1.Pod) sets.Set[string] {
	names := sets.New[string]()
	for _, p := range pods {
		names.Insert(p.Name)
	}
	return names
}

// text after the last helper that a regex over the function range would have picked up
func reconcile(m *v1alpha2.MondooAuditConfig) (string, error) {
	mergedSecretName := MergedSecretName(m)
	if len(mergedSecretName) > k8s.ResourceNameMaxLength {
		return "", nil
	}
	return mergedSecretName, nil
}