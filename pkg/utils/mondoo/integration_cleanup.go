// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	jwt "github.com/golang-jwt/jwt/v4"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/common"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
)

const cleanupTimeout = 30 * time.Second

// CleanupConsoleIntegration is called when a MondooAuditConfig is being deleted. It is
// best-effort and never blocks deletion: it reports the integration as DELETED to the Mondoo
// console (so it does not linger as ACTIVE forever) and, when the integration was created by
// the operator and .spec.consoleIntegration.deletionPolicy allows it, deletes the integration
// using the persisted provisioner credential.
func CleanupConsoleIntegration(
	ctx context.Context,
	kubeClient client.Client,
	mondooClientBuilder MondooClientBuilder,
	m *v1alpha2.MondooAuditConfig,
	httpProxy *string,
	httpsProxy *string,
	noProxy *string,
	log logr.Logger,
) {
	ctx, cancel := context.WithTimeout(ctx, cleanupTimeout)
	defer cancel()

	secret, err := k8s.GetIntegrationSecretForAuditConfig(ctx, kubeClient, *m)
	if err != nil {
		// no creds secret (never provisioned or already cleaned up) — nothing to do
		return
	}

	integrationMrn := strings.TrimSpace(string(secret.Data[constants.MondooCredsSecretIntegrationMRNKey]))
	if integrationMrn == "" {
		return
	}

	// Mark the integration DELETED in the console. This needs no extra permissions and
	// matters even when we do not delete the integration: K8s integrations are otherwise
	// shown as ACTIVE indefinitely once check-ins stop.
	if sa, err := k8s.GetServiceAccountFromSecret(*secret); err == nil {
		if token, err := GenerateTokenFromServiceAccount(*sa, log); err == nil {
			runtimeClient, err := mondooClientBuilder(mondooclient.MondooClientOptions{
				ApiEndpoint: sa.ApiEndpoint,
				Token:       token,
				HttpProxy:   httpProxy,
				HttpsProxy:  httpsProxy,
				NoProxy:     noProxy,
			})
			if err == nil {
				if err := runtimeClient.IntegrationReportStatus(ctx, &mondooclient.ReportStatusRequest{
					Mrn:    integrationMrn,
					Status: mondooclient.Status_DELETED,
				}); err != nil && !common.IsNotFound(err) {
					log.Error(err, "failed to report console integration as deleted", "integrationMRN", integrationMrn)
				}
			}
		}
	}

	if string(secret.Data[constants.MondooCredsSecretOperatorManagedKey]) != "true" {
		// the integration was not created by the operator — leave it alone
		return
	}
	if m.Spec.ConsoleIntegration.DeletionPolicy == v1alpha2.IntegrationDeletionPolicyRetain {
		log.Info("retaining operator-created console integration (deletionPolicy=Retain)", "integrationMRN", integrationMrn)
		return
	}

	cred := resolveProvisionerCredential(ctx, kubeClient, mondooClientBuilder, m, httpProxy, httpsProxy, noProxy, log)
	if cred == nil {
		log.Info("no provisioner credential available, leaving console integration in place",
			"integrationMRN", integrationMrn)
		return
	}

	token, err := GenerateTokenFromServiceAccount(cred.sa, log)
	if err != nil {
		log.Error(err, "unable to generate token from provisioner credential for integration cleanup")
		return
	}
	provisionerClient, err := mondooClientBuilder(mondooclient.MondooClientOptions{
		ApiEndpoint: cred.sa.ApiEndpoint,
		Token:       token,
		HttpProxy:   httpProxy,
		HttpsProxy:  httpsProxy,
		NoProxy:     noProxy,
	})
	if err != nil {
		log.Error(err, "failed to build Mondoo client for integration cleanup")
		return
	}

	if err := provisionerClient.IntegrationDelete(ctx, &mondooclient.IntegrationDeleteInput{Mrn: integrationMrn}); err != nil {
		if common.IsNotFound(err) {
			log.Info("console integration already deleted", "integrationMRN", integrationMrn)
			return
		}
		log.Error(err, "failed to delete console integration; it has been reported as DELETED but remains in the console",
			"integrationMRN", integrationMrn)
		return
	}
	log.Info("deleted operator-created console integration", "integrationMRN", integrationMrn)
}

// resolveProvisionerCredential loads the provisioner service account persisted during
// provisioning, falling back to the token Secret (either a service account JSON or a
// registration token that is exchanged on the fly).
func resolveProvisionerCredential(
	ctx context.Context,
	kubeClient client.Client,
	mondooClientBuilder MondooClientBuilder,
	m *v1alpha2.MondooAuditConfig,
	httpProxy *string,
	httpsProxy *string,
	noProxy *string,
	log logr.Logger,
) *provisionerCredential {
	provisionerSecret := &corev1.Secret{}
	key := client.ObjectKey{
		Name:      m.Spec.MondooCredsSecretRef.Name + constants.MondooProvisionerSecretSuffix,
		Namespace: m.Namespace,
	}
	if err := kubeClient.Get(ctx, key, provisionerSecret); err == nil {
		if cred, ok := parseServiceAccountCredential(string(provisionerSecret.Data[constants.MondooCredsSecretServiceAccountKey])); ok {
			return cred
		}
	}

	if m.Spec.MondooTokenSecretRef.Name == "" {
		return nil
	}
	tokenSecret := &corev1.Secret{}
	key = client.ObjectKey{Name: m.Spec.MondooTokenSecretRef.Name, Namespace: m.Namespace}
	if err := kubeClient.Get(ctx, key, tokenSecret); err != nil {
		return nil
	}
	tokenData := string(tokenSecret.Data[constants.MondooTokenSecretKey])
	if cred, ok := parseServiceAccountCredential(tokenData); ok {
		return cred
	}

	// registration token — exchange it for a (provisioner) service account
	jwtString := strings.TrimSpace(tokenData)
	if jwtString == "" {
		return nil
	}
	parser := &jwt.Parser{}
	token, _, err := parser.ParseUnverified(jwtString, jwt.MapClaims{})
	if err != nil {
		return nil
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil
	}
	mClient, err := mondooClientBuilder(mondooclient.MondooClientOptions{
		ApiEndpoint: strings.TrimSpace(strings.TrimRight(toString(claims["api_endpoint"]), "/")),
		Token:       jwtString,
		HttpProxy:   httpProxy,
		HttpsProxy:  httpsProxy,
		NoProxy:     noProxy,
	})
	if err != nil {
		return nil
	}
	resp, err := mClient.ExchangeRegistrationToken(ctx, &mondooclient.ExchangeRegistrationTokenInput{Token: jwtString})
	if err != nil {
		log.Error(err, "failed to exchange token for integration cleanup")
		return nil
	}
	if cred, ok := parseServiceAccountCredential(resp.ServiceAccount); ok {
		return cred
	}
	return nil
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
