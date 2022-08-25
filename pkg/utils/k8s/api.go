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
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// VerifyAPI will query the underlying k8s cluster for the existence
// of the provided group/version.
func VerifyAPI(group, version string, log logr.Logger) (bool, error) {
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

	log.Info(fmt.Sprintf("%s/%s API verified", group, version))
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
