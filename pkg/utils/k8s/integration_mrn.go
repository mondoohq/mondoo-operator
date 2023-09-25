// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TryGetIntegrationMrnForAuditConfig tries to get the integration-mrn for a MondooAuditConfig. If ConsoleIntegration is disabled, no
// integration-mrn is returned but also no error.
func TryGetIntegrationMrnForAuditConfig(ctx context.Context, kubeClient client.Client, auditConfig v1alpha2.MondooAuditConfig) (string, error) {
	if !auditConfig.Spec.ConsoleIntegration.Enable {
		return "", nil
	}

	secret, err := GetIntegrationSecretForAuditConfig(ctx, kubeClient, auditConfig)
	if err != nil {
		return "", err
	}

	return GetIntegrationMrnFromSecret(*secret)
}

// GetIntegrationSecretForAuditConfig retrieves the MondooCredsSecretRef for the give MondooAuditConfig.
func GetIntegrationSecretForAuditConfig(ctx context.Context, kubeClient client.Client, auditConfig v1alpha2.MondooAuditConfig) (*corev1.Secret, error) {
	serviceAccountSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      auditConfig.Spec.MondooCredsSecretRef.Name,
			Namespace: auditConfig.Namespace,
		},
	}
	if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(serviceAccountSecret), serviceAccountSecret); err != nil {
		return nil, err
	}

	return serviceAccountSecret, nil
}

func GetIntegrationMrnFromSecret(secret corev1.Secret) (string, error) {
	integrationMrn, ok := secret.Data[constants.MondooCredsSecretIntegrationMRNKey]
	if !ok {
		err := fmt.Errorf("creds Secret %s/%s missing %s key with integration MRN data",
			secret.Namespace, secret.Name, constants.MondooCredsSecretIntegrationMRNKey)
		return "", err
	}
	return string(integrationMrn), nil
}

func GetServiceAccountFromSecret(secret corev1.Secret) (*mondooclient.ServiceAccountCredentials, error) {
	serviceAccount := &mondooclient.ServiceAccountCredentials{}
	if err := json.Unmarshal(secret.Data[constants.MondooCredsSecretServiceAccountKey], serviceAccount); err != nil {
		msg := "failed to unmarshal creds Secret"
		return nil, fmt.Errorf("%s: %s", msg, err)
	}
	return serviceAccount, nil
}
