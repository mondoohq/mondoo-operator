package k8s

import (
	"io/ioutil"
	"os"
)

// GetRunningNamespace will return the namespace the Pod is running under
// Can fake the returned value (useful for local testing) by setting MONDOO_NAMESPACE_OVERRIDE
func GetRunningNamespace() (string, error) {
	// To allow running the controller locally, we should be able to set a namespace
	// without relying on a serviceaccount being mounted in.
	env, exists := os.LookupEnv("MONDOO_NAMESPACE_OVERRIDE")
	if exists {
		return env, nil
	}
	// Else, check the namespace of the ServiceAccount the Pod is running in.
	namespaceBytes, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", err
	}

	return string(namespaceBytes), nil
}
