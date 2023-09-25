// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"context"
	"os"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	namespaceBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", err
	}

	return string(namespaceBytes), nil
}

// GetClusterUID will just attempt to get the 'kube-system' Namespace and return the UID of the resource
func GetClusterUID(ctx context.Context, kubeClient client.Client, log logr.Logger) (string, error) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
		},
	}
	if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace); err != nil {
		log.Error(err, "Failed to get cluster ID from kube-system Namespace")
		return "", err
	}
	clusterID := string(namespace.UID)
	return clusterID, nil
}
