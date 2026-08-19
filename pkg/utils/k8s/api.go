// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"strings"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	// ResourceNameMaxLength is the 52-char safety bound used for CronJob names so
	// generated Job names stay within the Kubernetes 63-char limit.
	ResourceNameMaxLength = 52

	// DaemonSetNameChildResourceSafeMaxLength keeps DaemonSet names within the
	// DNS-1123 label limit (63) while leaving room for child Pod generated names
	// (<daemonset-name>-<5 random chars>).
	DaemonSetNameChildResourceSafeMaxLength = 57
)

// VerifyAPI will query the underlying k8s cluster for the existence
// of the provided group/version.
func VerifyAPI(group, version string) (bool, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return false, err
	}

	k8s, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return false, err
	}

	gv := schema.GroupVersion{
		Group:   group,
		Version: version,
	}

	if err = discovery.ServerSupportsVersion(k8s, gv); err != nil {
		// The returned error is just a vanilla fmt.Errorf()...
		// so just check for the static part of the error string
		if strings.Contains(err.Error(), "server does not support API version") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func VerifyResourceExists(group, version, resource string, log logr.Logger) (bool, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "unable to get k8s config")
		return false, err
	}

	k8s, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "unable to create k8s client")
		return false, err
	}

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	exists, err := discovery.IsResourceEnabled(k8s, gvr)
	if err != nil {
		log.Error(err, "error while check whether resource exists", "gvr", gvr)
		return false, err
	}

	return exists, nil
}
