// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

//go:build ignore

// xgrep test fixture, not compiled. Run: xgrep test .xgrep/rules
package nodes

// --- built from input: flagged ----------------------------------------------------------------

func cronJobFromSprintf(m v1alpha2.MondooAuditConfig, node corev1.Node) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			// ruleid: mondoo-object-name-not-length-checked
			Name:      fmt.Sprintf("%s-node-%s", m.Name, node.Name),
			Namespace: m.Namespace,
		},
	}
}

func deploymentFromTemplate(n *DeploymentHandler) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: n.Mondoo.Namespace,
			// ruleid: mondoo-object-name-not-length-checked
			Name:      fmt.Sprintf(WorkloadDeploymentNameTemplate, n.Mondoo.Name),
		},
	}
}

func serviceFromConcat(m v1alpha2.MondooAuditConfig) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			// ruleid: mondoo-object-name-not-length-checked
			Name: m.Name + "-scan-api",
		},
	}
}

func serviceFromJoin(m v1alpha2.MondooAuditConfig) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			// ruleid: mondoo-object-name-not-length-checked
			Name: strings.Join([]string{m.GetName(), "scan-api"}, "-"),
		},
	}
}

func configMapFromCluster(m v1alpha2.MondooAuditConfig, cluster v1alpha2.ExternalCluster) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			// ruleid: mondoo-object-name-not-length-checked
			Name:      fmt.Sprintf("mondoo-k8s-external-%s-inventory", cluster.Name),
			Namespace: m.Namespace,
		},
	}
}

func secretFromConcat(m v1alpha2.MondooAuditConfig) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			// ruleid: mondoo-object-name-not-length-checked
			Name:      m.Name + ConfigOverrideSecretSuffix,
			Namespace: m.Namespace,
		},
	}
}

func secretFromUserSecret(serviceAccountSecret corev1.Secret) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			// ruleid: mondoo-object-name-not-length-checked
			Name: serviceAccountSecret.Name + constants.MondooProvisionerSecretSuffix,
		},
	}
}

func webhookFromNamespaceAndName(m v1alpha2.MondooAuditConfig) *admissionregistrationv1.ValidatingWebhookConfiguration {
	webhookName := fmt.Sprintf("%s-%s-mondoo", m.Namespace, m.Name)
	return &admissionregistrationv1.ValidatingWebhookConfiguration{
		// ruleid: mondoo-object-name-not-length-checked
		ObjectMeta: metav1.ObjectMeta{Name: webhookName},
	}
}

func valueLiteralNotPointer(m v1alpha2.MondooAuditConfig) corev1.ServiceAccount {
	return corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			// ruleid: mondoo-object-name-not-length-checked
			Name: strings.ToLower(m.Name + "-wif"),
		},
	}
}

// --- length-checked: not flagged ----------------------------------------------------------------

func cronJobViaPrimitive(m v1alpha2.MondooAuditConfig) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			// ok: mondoo-object-name-not-length-checked
			Name:      k8s.CronJobName("nodes", m.Name),
			Namespace: m.Namespace,
		},
	}
}

func cronJobViaQualifiedHelper(auditConfig *v1alpha2.MondooAuditConfig, cluster v1alpha2.ExternalCluster) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			// ok: mondoo-object-name-not-length-checked
			Name:      k8s_scan.ExternalClusterCronJobName(auditConfig.Name, cluster.Name),
			Namespace: auditConfig.Namespace,
		},
	}
}

// a helper is trusted by its name here; name-helper-without-length-check checks its body
func deploymentViaHelper(m v1alpha2.MondooAuditConfig) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			// ok: mondoo-object-name-not-length-checked
			Name:      DeploymentName(m.Name),
			Namespace: m.Namespace,
		},
	}
}

func configMapViaHelperWithNode(m v1alpha2.MondooAuditConfig, node corev1.Node) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			// ok: mondoo-object-name-not-length-checked
			Name:      ConfigMapNameWithNode(m.Name, node.Name),
			Namespace: m.Namespace,
		},
	}
}

func sprintfOverHelperOutput(m v1alpha2.MondooAuditConfig) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			// ok: mondoo-object-name-not-length-checked
			Name: fmt.Sprintf("%s", ConfigMapName(m.Name)),
		},
	}
}

func daemonSetCopiesConstructedName(m v1alpha2.MondooAuditConfig, image string) *appsv1.DaemonSet {
	desired := DaemonSet(m, image)
	// ok: mondoo-object-name-not-length-checked
	return &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
}

func lookupByUserSuppliedRef(ctx context.Context, c client.Client, m v1alpha2.MondooAuditConfig) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			// ok: mondoo-object-name-not-length-checked
			Name:      m.Spec.MondooCredsSecretRef.Name,
			Namespace: m.Namespace,
		},
	}
	return c.Get(ctx, client.ObjectKeyFromObject(secret), secret)
}

func sameObjectLookup(serviceAccountSecret *corev1.Secret) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			// ok: mondoo-object-name-not-length-checked
			Name:      serviceAccountSecret.Name,
			Namespace: serviceAccountSecret.Namespace,
		},
	}
}

// a workload named after the raw input, with more than the identity in its ObjectMeta: flagged
func cronJobNamedAfterInput(m v1alpha2.MondooAuditConfig) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			// ruleid: mondoo-object-name-not-length-checked
			Name:      m.Name,
			Namespace: m.Namespace,
			Labels:    map[string]string{"app": "mondoo"},
		},
	}
}

func cronJobNamedAfterNode(node corev1.Node) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			// ruleid: mondoo-object-name-not-length-checked
			Name: node.Name,
		},
	}
}

func constantName(m v1alpha2.MondooAuditConfig) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			// ok: mondoo-object-name-not-length-checked
			Name:      "mondoo-operator-k8s-resources-scanning",
			Namespace: m.Namespace,
		},
	}
}

// a same-file helper: xgrep follows the call and would hand m.Name's taint to the caller
func MergedSecretName(m *v1alpha2.MondooAuditConfig) string {
	return m.Name + MergedPrivateRegistriesSecretSuffix
}

func secretViaSameFileHelper(m *v1alpha2.MondooAuditConfig) *corev1.Secret {
	mergedSecretName := MergedSecretName(m)
	// ok: mondoo-object-name-not-length-checked
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: mergedSecretName, Namespace: m.Namespace}}
}