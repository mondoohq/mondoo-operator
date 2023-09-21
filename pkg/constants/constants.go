// Copyright (c) Mondoo, Inc.
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
)
