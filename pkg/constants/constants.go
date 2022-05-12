package constants

const (
	// MondooCredsSecretServiceAccountKey is the name of the key in the Secret that holds the
	// Mondoo service account data
	MondooCredsSecretServiceAccountKey = "config"
	// MondooCredsSecretIntegrationMRNKey is the name of the key in the Secret that holds the
	// Mondoo integration MRN (used to CheckIn() via the Mondoo API)
	MondooCredsSecretIntegrationMRNKey = "integraionmrn"
	// MondooTokenSecretKey is the name of the key in the Secret that holds the JWT data
	// used for creating a Mondoo service account
	MondooTokenSecretKey = "token"
)
