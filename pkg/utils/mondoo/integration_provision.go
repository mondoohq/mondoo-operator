// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package mondoo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/common"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
)

const (
	// integrationMrnSegment identifies integration MRNs, e.g.
	// //integration.api.mondoo.app/spaces/<space>/integrations/<id>
	integrationMrnSegment = "/integrations/"

	// clusterUIDIdentifierPrefix marks the identifier holding the cluster UID on
	// operator-created integrations, so all integrations of one cluster can be found.
	clusterUIDIdentifierPrefix = "k8s-cluster-uid:"
	// auditConfigIdentifierPrefix marks the identifier that uniquely ties an integration to
	// one MondooAuditConfig in one cluster. It is the operator's idempotency key: only the
	// operator ever sets it, so an integration carrying it is operator-created by definition.
	auditConfigIdentifierPrefix = "k8s-audit-config:"

	createdByLabelKey   = "mondoo.com/created-by"
	createdByLabelValue = "mondoo-operator"
)

// provisionerCredential is a Mondoo credential that is allowed to manage integrations in a
// space (e.g. carrying the deployment-manager role), used to create or re-attach to the
// console integration. It is distinct from the runtime (agent-role) service account the
// operator scans with.
type provisionerCredential struct {
	sa mondooclient.ServiceAccountCredentials
	// raw is the original service account JSON, persisted for later lifecycle operations
	// (e.g. deleting the integration when the MondooAuditConfig is deleted).
	raw string
}

// parseServiceAccountCredential detects a Mondoo service account JSON in the token Secret
// (as opposed to a JWT registration token).
func parseServiceAccountCredential(tokenSecretData string) (*provisionerCredential, bool) {
	trimmed := strings.TrimSpace(tokenSecretData)
	if !strings.HasPrefix(trimmed, "{") {
		return nil, false
	}
	sa := mondooclient.ServiceAccountCredentials{}
	if err := json.Unmarshal([]byte(trimmed), &sa); err != nil {
		return nil, false
	}
	if sa.Mrn == "" || sa.PrivateKey == "" || sa.ApiEndpoint == "" {
		return nil, false
	}
	return &provisionerCredential{sa: sa, raw: trimmed}, true
}

// loadPersistedProvisionerCredential returns the provisioner credential saved by an earlier
// provisioning attempt. Reusing it keeps retries from exchanging the registration token again
// and again, which would mint a new server-side service account each time.
func loadPersistedProvisionerCredential(ctx context.Context, kubeClient client.Client, serviceAccountSecret types.NamespacedName) (*provisionerCredential, bool) {
	secret := &corev1.Secret{}
	key := client.ObjectKey{
		Name:      serviceAccountSecret.Name + constants.MondooProvisionerSecretSuffix,
		Namespace: serviceAccountSecret.Namespace,
	}
	if err := kubeClient.Get(ctx, key, secret); err != nil {
		return nil, false
	}
	return parseServiceAccountCredential(string(secret.Data[constants.MondooCredsSecretServiceAccountKey]))
}

// IsIntegrationMrn returns whether the given MRN (typically the "owner" claim of a
// registration token) refers to a console integration.
func IsIntegrationMrn(mrn string) bool {
	return strings.Contains(mrn, integrationMrnSegment)
}

// AuditConfigIdentifier returns the identifier that uniquely ties a console integration to
// the given MondooAuditConfig in the given cluster.
func AuditConfigIdentifier(clusterUID string, auditConfig client.ObjectKey) string {
	return fmt.Sprintf("%s%s/%s/%s", auditConfigIdentifierPrefix, clusterUID, auditConfig.Namespace, auditConfig.Name)
}

func integrationName(m *v1alpha2.MondooAuditConfig, clusterUID string) string {
	if m.Spec.ConsoleIntegration.Name != "" {
		return m.Spec.ConsoleIntegration.Name
	}
	suffix := clusterUID
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	return "mondoo-operator-" + suffix
}

// integrationScopeMrn determines the space the integration is created in: .spec.spaceId when
// set, otherwise the space the provisioner credential is scoped to.
func integrationScopeMrn(m *v1alpha2.MondooAuditConfig, cred *provisionerCredential) (string, error) {
	credScope := cred.sa.ScopeMrn
	if credScope == "" {
		credScope = cred.sa.SpaceMrn
	}

	specScope := k8s.SpaceMrnForAuditConfig(*m)
	if specScope != "" {
		if strings.Contains(credScope, "/spaces/") && credScope != specScope {
			return "", fmt.Errorf(
				"the provided credential is scoped to %q which does not match .spec.spaceId (%q)", credScope, specScope)
		}
		return specScope, nil
	}

	if strings.Contains(credScope, "/spaces/") {
		return credScope, nil
	}
	return "", fmt.Errorf(
		"cannot determine the target space for the console integration: the credential is scoped to %q; set .spec.spaceId to select a space", credScope)
}

// k8sConfigurationOptions derives the integration's scan configuration from the
// MondooAuditConfig so the console shows what the operator actually does. This is a
// create-time snapshot; later spec changes are not synced back to the console.
func k8sConfigurationOptions(m *v1alpha2.MondooAuditConfig) *mondooclient.K8sConfigurationOptionsInput {
	opts := &mondooclient.K8sConfigurationOptionsInput{
		ScanNodes:          m.Spec.Nodes.Enable,
		ScanWorkloads:      m.Spec.KubernetesResources.Enable,
		ScanPublicImages:   m.Spec.Containers.Enable,
		ScanLocalCluster:   true,
		NamespaceAllowList: m.Spec.Filtering.Namespaces.Include,
		NamespaceDenyList:  m.Spec.Filtering.Namespaces.Exclude,
		Schedule:           m.Spec.KubernetesResources.Schedule,
		NodesSchedule:      m.Spec.Nodes.Schedule,
		ContainersSchedule: m.Spec.Containers.Schedule,
	}
	if m.Spec.Nodes.Enable {
		switch m.Spec.Nodes.Style {
		case v1alpha2.NodeScanStyle_Deployment:
			opts.ScanNodesStyle = mondooclient.ScanNodesStyleDeployment
		case v1alpha2.NodeScanStyle_DaemonSet:
			opts.ScanNodesStyle = mondooclient.ScanNodesStyleDaemonSet
		default:
			opts.ScanNodesStyle = mondooclient.ScanNodesStyleCronJob
		}
	}
	return opts
}

// provisionConsoleIntegration creates the console integration for the MondooAuditConfig (or
// re-attaches to one created earlier, found via its audit-config identifier), registers a
// runtime service account with the integration's own registration token and persists it to
// the creds Secret — marked as operator-managed so it can be cleaned up on deletion. The
// provisioner credential is persisted to a companion Secret for those lifecycle operations.
func provisionConsoleIntegration(
	ctx context.Context,
	kubeClient client.Client,
	mondooClientBuilder MondooClientBuilder,
	m *v1alpha2.MondooAuditConfig,
	cred *provisionerCredential,
	serviceAccountSecret types.NamespacedName,
	httpProxy *string,
	httpsProxy *string,
	noProxy *string,
	log logr.Logger,
) error {
	clusterUID, err := k8s.GetClusterUID(ctx, kubeClient, log)
	if err != nil {
		return fmt.Errorf("failed to determine cluster UID for integration provisioning: %w", err)
	}

	scopeMrn, err := integrationScopeMrn(m, cred)
	if err != nil {
		return err
	}

	provisionerToken, err := GenerateTokenFromServiceAccount(cred.sa, log)
	if err != nil {
		return fmt.Errorf("unable to generate token from provisioner credential: %w", err)
	}

	provisionerClient, err := mondooClientBuilder(mondooclient.MondooClientOptions{
		ApiEndpoint: cred.sa.ApiEndpoint,
		Token:       provisionerToken,
		HttpProxy:   httpProxy,
		HttpsProxy:  httpsProxy,
		NoProxy:     noProxy,
	})
	if err != nil {
		return err
	}

	// Persist the provisioner credential before doing anything with it: a retry after a
	// partial failure then reuses this credential instead of re-exchanging the registration
	// token (each exchange mints another service account server-side), and integration
	// cleanup on deletion depends on it. Owner-referenced, so it is garbage collected with
	// the CR.
	provisionerSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountSecret.Name + constants.MondooProvisionerSecretSuffix,
			Namespace: serviceAccountSecret.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, kubeClient, provisionerSecret, func() error {
		if err := controllerutil.SetControllerReference(m, provisionerSecret, kubeClient.Scheme()); err != nil {
			return err
		}
		provisionerSecret.Data = map[string][]byte{
			constants.MondooCredsSecretServiceAccountKey: []byte(cred.raw),
		}
		return nil
	}); err != nil {
		// provisioning can continue; retries may re-exchange and deletion cleanup will fall
		// back to the token Secret
		log.Error(err, "failed to save provisioner credential secret")
	}

	identifier := AuditConfigIdentifier(clusterUID, client.ObjectKeyFromObject(m))

	list, err := provisionerClient.IntegrationList(ctx, &mondooclient.IntegrationListInput{
		ScopeMrn:    scopeMrn,
		Types:       []string{mondooclient.IntegrationTypeK8s},
		Identifiers: []string{identifier},
	})
	if err != nil {
		if common.IsForbidden(err) {
			return fmt.Errorf(
				"the provided credential is not allowed to manage integrations in %s; "+
					"use a credential with the deployment-manager (or editor) role, or set .spec.consoleIntegration.autoCreate=false: %w", scopeMrn, err)
		}
		return fmt.Errorf("failed to look up existing console integrations: %w", err)
	}

	var integrationMrn, integrationToken string
	switch len(list.Integrations) {
	case 0:
		resp, err := provisionerClient.IntegrationCreate(ctx, &mondooclient.IntegrationCreateInput{
			ScopeMrn:    scopeMrn,
			Name:        integrationName(m, clusterUID),
			Type:        mondooclient.IntegrationTypeK8s,
			Identifiers: []string{identifier, clusterUIDIdentifierPrefix + clusterUID},
			// the token is used immediately below, no need for a non-expiring one
			LongLivedToken: false,
			Labels:         map[string]string{createdByLabelKey: createdByLabelValue},
			ConfigurationInput: &mondooclient.IntegrationConfigurationInput{
				K8sOptions: k8sConfigurationOptions(m),
			},
		})
		if err != nil {
			if common.IsForbidden(err) {
				return fmt.Errorf(
					"the provided credential is not allowed to create integrations in %s; "+
						"use a credential with the deployment-manager (or editor) role, or set .spec.consoleIntegration.autoCreate=false: %w", scopeMrn, err)
			}
			return fmt.Errorf("failed to create console integration: %w", err)
		}
		if resp.Integration == nil || resp.Integration.Mrn == "" || resp.Integration.Token == "" {
			return fmt.Errorf("console integration creation returned an incomplete response")
		}
		integrationMrn = resp.Integration.Mrn
		integrationToken = resp.Integration.Token
		log.Info("created Mondoo console integration", "integrationMRN", integrationMrn, "scope", scopeMrn)
	case 1:
		integrationMrn = list.Integrations[0].Mrn
		tokenResp, err := provisionerClient.IntegrationGetToken(ctx, &mondooclient.IntegrationGetTokenInput{
			Mrn:            integrationMrn,
			LongLivedToken: false,
		})
		if err != nil {
			return fmt.Errorf("failed to get token for existing console integration %s: %w", integrationMrn, err)
		}
		if tokenResp.Token == "" {
			return fmt.Errorf("getting a token for existing console integration %s returned an empty token", integrationMrn)
		}
		integrationToken = tokenResp.Token
		log.Info("re-attaching to existing Mondoo console integration", "integrationMRN", integrationMrn)
	default:
		mrns := make([]string, 0, len(list.Integrations))
		for _, i := range list.Integrations {
			mrns = append(mrns, i.Mrn)
		}
		return fmt.Errorf(
			"found %d console integrations for this MondooAuditConfig (%s); delete the duplicates in the Mondoo console and retry",
			len(list.Integrations), strings.Join(mrns, ", "))
	}

	// Register with the integration's own token (not the provisioner credential) so the
	// runtime service account gets the default agent role instead of integration-management
	// permissions.
	registerClient, err := mondooClientBuilder(mondooclient.MondooClientOptions{
		ApiEndpoint: cred.sa.ApiEndpoint,
		Token:       integrationToken,
		HttpProxy:   httpProxy,
		HttpsProxy:  httpsProxy,
		NoProxy:     noProxy,
	})
	if err != nil {
		return err
	}

	registerResp, err := registerClient.IntegrationRegister(ctx, &mondooclient.IntegrationRegisterInput{
		Mrn:   integrationMrn,
		Token: integrationToken,
	})
	if err != nil {
		return fmt.Errorf("failed to register with console integration %s: %w", integrationMrn, err)
	}
	if registerResp.Creds == nil {
		return fmt.Errorf("registering with console integration %s returned no credentials", integrationMrn)
	}

	credsBytes, err := json.Marshal(*registerResp.Creds) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed to marshal service account creds from IntegrationRegister(): %w", err)
	}

	// CreateOrUpdate (instead of create-if-not-exists) so leftovers from a partially failed
	// earlier attempt cannot pin stale credentials or a stale integration MRN.
	credsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountSecret.Name,
			Namespace: serviceAccountSecret.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, kubeClient, credsSecret, func() error {
		credsSecret.Data = map[string][]byte{
			constants.MondooCredsSecretServiceAccountKey:  credsBytes,
			constants.MondooCredsSecretIntegrationMRNKey:  []byte(integrationMrn),
			constants.MondooCredsSecretOperatorManagedKey: []byte("true"),
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error while trying to save Mondoo service account into secret: %w", err)
	}

	log.Info("saved Mondoo service account for console integration",
		"secret", fmt.Sprintf("%s/%s", serviceAccountSecret.Namespace, serviceAccountSecret.Name),
		"integrationMRN", integrationMrn)

	// No easy way to retry this one-off CheckIn(). An error on initial CheckIn()
	// means we'll just retry on the regularly scheduled interval via the integration controller
	_ = performInitialCheckIn(ctx, mondooClientBuilder, integrationMrn, *registerResp.Creds, httpProxy, httpsProxy, noProxy, log)

	return nil
}
