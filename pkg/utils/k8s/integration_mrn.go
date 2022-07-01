/*
Copyright 2022 Mondoo, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetIntegrationSecretForAuditConfig retrieves the integration-mrn and service account for a MondooAuditConfig. This is only relevant if ConsoleIntegration
// is enabled. When ConsoleIntegration is disabled an empty string is returned.
func GetIntegrationSecretForAuditConfig(ctx context.Context, kubeClient client.Client, auditConfig v1alpha2.MondooAuditConfig) (string, *mondooclient.ServiceAccountCredentials, error) {
	if !auditConfig.Spec.ConsoleIntegration.Enable {
		// sending an empty integrationMRN means the webhook or k8s resources scan will run w/o setting integration
		// labels (which is exactly what we want when console integration is not enabled)
		return "", nil, nil
	}

	serviceAccountSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      auditConfig.Spec.MondooCredsSecretRef.Name,
			Namespace: auditConfig.Namespace,
		},
	}
	if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(serviceAccountSecret), serviceAccountSecret); err != nil {
		return "", nil, err
	}
	integrationMrn, ok := serviceAccountSecret.Data[constants.MondooCredsSecretIntegrationMRNKey]
	if !ok {
		err := fmt.Errorf("creds Secret %s/%s missing %s key with integration MRN data",
			serviceAccountSecret.Namespace, serviceAccountSecret.Name, constants.MondooCredsSecretIntegrationMRNKey)
		return "", nil, err
	}

	serviceAccount := &mondooclient.ServiceAccountCredentials{}
	if err := json.Unmarshal(serviceAccountSecret.Data[constants.MondooCredsSecretServiceAccountKey], serviceAccount); err != nil {
		msg := "failed to unmarshal creds Secret"
		return "", nil, fmt.Errorf("%s: %s", msg, err)
	}

	return string(integrationMrn), serviceAccount, nil
}
