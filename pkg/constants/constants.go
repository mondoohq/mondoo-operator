// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package constants

const (
	// MondooCredsSecretServiceAccountKey is the name of the key in the Secret that holds the
	// Mondoo service account data
	MondooCredsSecretServiceAccountKey = "config"
	// MondooCredsSecretIntegrationMRNKey is the name of the key in the Secret that holds the
	// Mondoo integration MRN (used to CheckIn() via the Mondoo API)
	MondooCredsSecretIntegrationMRNKey = "integrationmrn"
	// MondooTokenSecretKey is the name of the key in the Secret that holds the JWT data
	// used for creating a Mondoo service account
	MondooTokenSecretKey = "token"
	// MondooAssetsIntegrationLabel is the label we set for any assets whenever the consoleIntegration is enabled
	// (for consistency with other integrations, the integration tag will not use the 'k8s' prefix)
	MondooAssetsIntegrationLabel = "mondoo.com/" + "integration-mrn"

	// Operator-managed asset annotations propagated to all discovered assets for routing.
	MondooAuditConfigAnnotation          = "mondoo.com/audit-config/name"
	MondooAuditConfigNamespaceAnnotation = "mondoo.com/audit-config/namespace"
	MondooClusterNameAnnotation          = "mondoo.com/audit-config/cluster-name"
)

// AuditConfigAnnotations returns operator-managed annotations identifying the
// MondooAuditConfig that owns the scan. These are propagated to all discovered
// assets so that server-side routing rules can match on them.
func AuditConfigAnnotations(name, namespace string) map[string]string {
	return map[string]string{
		MondooAuditConfigAnnotation:          name,
		MondooAuditConfigNamespaceAnnotation: namespace,
	}
}
