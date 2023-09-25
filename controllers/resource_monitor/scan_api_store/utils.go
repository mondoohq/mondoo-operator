// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package scan_api_store

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers/scanapi"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HandleAuditConfig adds the scan API service URL, token and integration MRN to the scan API store if the provided MondooAuditConfig has k8s
// resources enabled.
func HandleAuditConfig(ctx context.Context, kubeClient client.Client, scanApiStore ScanApiStore, auditConfig v1alpha2.MondooAuditConfig) error {
	if auditConfig.Spec.KubernetesResources.Enable {
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: scanapi.TokenSecretName(auditConfig.Name), Namespace: auditConfig.Namespace}}
		if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
			return err
		}
		integrationMrn, err := k8s.TryGetIntegrationMrnForAuditConfig(ctx, kubeClient, auditConfig)
		if err != nil {
			return err
		}

		opts := &ScanApiStoreAddOpts{
			Url:               scanapi.ScanApiServiceUrl(auditConfig),
			Token:             string(secret.Data[constants.MondooTokenSecretKey]),
			IntegrationMrn:    integrationMrn,
			IncludeNamespaces: auditConfig.Spec.Filtering.Namespaces.Include,
			ExcludeNamespaces: auditConfig.Spec.Filtering.Namespaces.Exclude,
		}
		scanApiStore.Add(opts)
	}
	return nil
}
