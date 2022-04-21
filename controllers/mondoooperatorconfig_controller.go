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

package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mondoov1alpha1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
)

// MondooOperatorConfigReconciler reconciles a MondooOperatorConfig object
type MondooOperatorConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondoooperatorconfigs,verbs=get;watch
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondoooperatorconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondoooperatorconfigs/finalizers,verbs=update

// Reconcile will check for a valid MondooOperatorConfig resource (only "mondoo-operator-config" allowed), and
// set up the mondoo-operator as indicated in the resource.
func (r *MondooOperatorConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	configLog := log.FromContext(ctx)

	// Since MondooOperatorConfig is cluster-scoped, there should only ever be one resource
	// for configuring the mondoo-operator. Ensure the name of the resource is what we expect.
	if req.Name != mondoov1alpha1.MondooOperatorConfigName {
		configLog.Info(fmt.Sprintf("only a single MondooOperatorConfig can be used to configure mondoo-operator and it must be named %s", mondoov1alpha1.MondooOperatorConfigName))
		return ctrl.Result{}, nil
	}

	config := &mondoov1alpha1.MondooOperatorConfig{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		if errors.IsNotFound(err) {
			configLog.Info("MondooOperatorConfig no longer exists")
			return ctrl.Result{}, nil
		}
		configLog.Error(err, "failed to get MondooOperatorConfig")
		return ctrl.Result{}, err
	}

	if config.DeletionTimestamp != nil {
		// Object being deleted; nothing to do
		return ctrl.Result{}, nil
	}

	namespace, err := k8s.GetRunningNamespace()
	if err != nil {
		configLog.Error(err, "failed to know which namespace to target")
		return ctrl.Result{}, err
	}
	serviceMonitor := ServiceMonitor{
		Config:          config,
		TargetNamespace: namespace,
	}
	result, err := serviceMonitor.Reconcile(ctx, r.Client, r.Scheme, req)
	if err != nil {
		configLog.Error(err, "Failed to set up serviceMonitor")
	}
	if err != nil || result.Requeue {
		return result, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MondooOperatorConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mondoov1alpha1.MondooOperatorConfig{}).
		Complete(r)
}
