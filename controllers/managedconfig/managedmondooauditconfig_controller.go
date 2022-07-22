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

package managedconfig

import (
	"context"
	_ "embed"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
)

// ManagedMondooAuditConfigReconciler reconciles a ManagedMondooAuditConfig object
type ManagedMondooAuditConfigReconciler struct {
	client.Client
	MondooClientBuilder func(mondooclient.ClientOptions) mondooclient.Client
	// StatusReporter         *status.StatusReporter
}

// so we can mock out the mondoo client for testing
var MondooClientBuilder = mondooclient.NewClient

// The update permissions for MondooAuditConfigs are required because having update permissions just for finalizers is insufficient
// to add finalizers. There is a github issue describing the problem https://github.com/kubernetes-sigs/kubebuilder/issues/2264
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooauditconfigs,verbs=get;list;watch;update
// Need to be able to check for the existence of Secrets with tokens, Mondoo service accounts, and private image pull secrets without asking for permission to read all Secrets
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get,resourceNames=mondoo-client;mondoo-token;mondoo-private-registries-secrets

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ManagedMondooAuditConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (reconcileResult ctrl.Result, reconcileError error) {
	log := ctrllog.FromContext(ctx)

	// Fetch the Mondoo instance
	managedConfig := &v1alpha2.ManagedMondooAuditConfig{}

	reconcileError = r.Get(ctx, req.NamespacedName, managedConfig)

	if reconcileError != nil {
		if errors.IsNotFound(reconcileError) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("mondoo resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(reconcileError, "Failed to get mondoo")
		return ctrl.Result{}, reconcileError
	}

	config := &v1alpha2.MondooOperatorConfig{}
	if reconcileError = r.Get(ctx, types.NamespacedName{Name: v1alpha2.MondooOperatorConfigName}, config); reconcileError != nil {
		if errors.IsNotFound(reconcileError) {
			log.Info("MondooOperatorConfig not found, using defaults")
		} else {
			log.Error(reconcileError, "Failed to check for MondooOpertorConfig")
			return ctrl.Result{}, reconcileError
		}
	}

	/*
		defer func() {
			reportErr := r.StatusReporter.Report(ctx, *mondooAuditConfig)

			// If the err from the reconcile func is nil, the all steps were executed it successfully
			// If there was an error, we do not override the existing error with the status report error
			if reconcileError == nil {
				reconcileError = reportErr
			}
		}()

		if !config.DeletionTimestamp.IsZero() {
			// Going to proceed as if there is no MondooOperatorConfig
			config = &v1alpha2.MondooOperatorConfig{}
		}
	*/

	/*
		managedConfigCopy := managedConfig.DeepCopy()

		// If spec.MondooTokenSecretRef != "" and the Secret referenced in spec.MondooCredsSecretRef
		// does not exist, then attempt to trade the token for a Mondoo service account and save it
		// in the Secret referenced in .spec.MondooCredsSecretRef
		if reconcileError = r.exchangeTokenForServiceAccount(ctx, mondooAuditConfig, log); reconcileError != nil {
			log.Error(reconcileError, "errors while checking if Mondoo service account needs creating")
			return ctrl.Result{}, reconcileError
		}

		// Update status.ReconciledByOperatorVersion to the running operator version
		// This should only happen, after all objects have been reconciled
		////r.MondooAuditConfig.Status.ReconciledByOperatorVersion = version.Version
	*/

	return ctrl.Result{Requeue: true, RequeueAfter: time.Hour * 24 * 7}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedMondooAuditConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.ManagedMondooAuditConfig{}).
		Complete(r)
}

// labelsForMondoo returns the labels for selecting the resources
// belonging to the given mondoo CR name.
func labelsForMondoo(name string) map[string]string {
	return map[string]string{"mondoo_cr": name}
}
