/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package resource_monitor

import (
	"context"
	"fmt"
	"strings"

	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var logger = log.Log.WithName("resource_monitor")

type ResourceMonitorController struct {
	client.Client
	createRes           func() client.Object
	debouncer           *debouncer
	resourceType        string
	mondooClientBuilder func(mondooclient.ClientOptions) mondooclient.Client
}

func NewResourceMonitorController(kubeClient client.Client, createRes func() client.Object) *ResourceMonitorController {
	gvk, err := apiutil.GVKForObject(createRes(), kubeClient.Scheme())
	if err != nil {
		logger.Error(err, "Failed to get GVK for resource") // This should never happen in practice
		panic(err)
	}

	return &ResourceMonitorController{
		Client:              kubeClient,
		createRes:           createRes,
		debouncer:           NewDebouncer(),
		resourceType:        strings.ToLower(gvk.Kind),
		mondooClientBuilder: mondooclient.NewClient,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ResourceMonitorController) SetupWithManager(mgr ctrl.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(r.createRes()).
		WithEventFilter(k8s.CreateUpdateEventsPredicate{}).
		Complete(r); err != nil {
		return err
	}
	go r.debouncer.Start()
	return nil
}

func (r *ResourceMonitorController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.debouncer.Add(fmt.Sprintf("%s:%s:%s", r.resourceType, req.Namespace, req.Name))
	return ctrl.Result{}, nil
}
