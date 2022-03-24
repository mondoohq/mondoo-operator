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
	_ "embed"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.mondoo.com/mondoo-operator/api/v1alpha1"
	mondoov1alpha1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	finalizerString       = "k8s.mondoo.com/delete"
	defaultServiceAccount = "mondoo-operator-workload"
)

// MondooAuditConfigReconciler reconciles a MondooAuditConfig object
type MondooAuditConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Embed the Default Inventory for Daemonset and Deployment Configurations
//go:embed inventory-ds.yaml
var dsInventoryyaml []byte

//go:embed inventory-deploy.yaml
var deployInventoryyaml []byte

//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooauditconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooauditconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooauditconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cert-manager.io,resources=certificates;issuers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch
//The last line is required as we cant assign higher permissions that exist for operator serviceaccount

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MondooAuditConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *MondooAuditConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	// Fetch the Mondoo instance
	mondoo := &v1alpha1.MondooAuditConfig{}

	err := r.Get(ctx, req.NamespacedName, mondoo)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("mondoo resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get mondoo")
		return ctrl.Result{}, err
	}
	config := &mondoov1alpha1.MondooOperatorConfig{}
	if err := r.Get(ctx, types.NamespacedName{Name: mondoov1alpha1.MondooOperatorConfigName}, config); err != nil {
		if errors.IsNotFound(err) {
			log.Info("MondooOperatorConfig not found, using defaults")
		}
		log.Info(err.Error())
	}

	if config.DeletionTimestamp != nil {
		// Object being deleted; nothing to do
		return ctrl.Result{}, nil
	}

	if mondoo.DeletionTimestamp != nil {
		log.Info("deleting")

		// Any other Reconcile() loops that need custom cleanup when the MondooAuditConfig is being
		// deleted should be called here

		webhooks := Webhooks{
			Mondoo:               mondoo,
			KubeClient:           r.Client,
			TargetNamespace:      req.Namespace,
			Scheme:               r.Scheme,
			MondooOperatorConfig: *config,
		}
		result, err := webhooks.Reconcile(ctx)
		if err != nil {
			log.Error(err, "failed to cleanup webhooks")
			return result, err
		}

		removeFinalizer(mondoo)
		if err := r.Update(ctx, mondoo); err != nil {
			log.Error(err, "failed to remove finalizer")
		}
		return ctrl.Result{}, err
	} else {
		if !hasFinalizer(mondoo) {
			addFinalizer(mondoo)
			if err := r.Update(ctx, mondoo); err != nil {
				log.Error(err, "failed to set finalizer")
			}
			return ctrl.Result{}, err
		}
	}

	nodes := Nodes{
		Enable:               mondoo.Spec.Nodes.Enable,
		Mondoo:               *mondoo,
		MondooOperatorConfig: *config,
	}

	result, err := nodes.Reconcile(ctx, r.Client, r.Scheme, req, string(dsInventoryyaml))
	if err != nil {
		log.Error(err, "Failed to declare nodes")
	}
	if err != nil || result.Requeue {
		return result, err
	}

	workloads := Workloads{
		Enable:               mondoo.Spec.Workloads.Enable,
		Mondoo:               *mondoo,
		MondooOperatorConfig: *config,
	}

	result, err = workloads.Reconcile(ctx, r.Client, r.Scheme, req, string(deployInventoryyaml))
	if err != nil {
		log.Error(err, "Failed to declare workloads")
	}
	if err != nil || result.Requeue {
		return result, err
	}

	webhooks := Webhooks{
		Mondoo:               mondoo,
		KubeClient:           r.Client,
		TargetNamespace:      req.Namespace,
		Scheme:               r.Scheme,
		MondooOperatorConfig: *config,
	}

	result, err = webhooks.Reconcile(ctx)
	if err != nil {
		log.Error(err, "Failed to set up webhooks")
	}
	if err != nil || result.Requeue {
		return result, err
	}

	// Update the mondoo status with the pod names only after all pod creation actions are done
	// List the pods for this mondoo's daemonset and deployment
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(mondoo.Namespace),
		client.MatchingLabels(labelsForMondoo(mondoo.Name)),
	}
	if err = r.List(ctx, podList, listOpts...); err != nil {
		log.Error(err, "Failed to list pods", "Mondoo.Namespace", mondoo.Namespace, "Mondoo.Name", mondoo.Name)
		return ctrl.Result{}, err
	}
	podListNames := getPodNames(podList.Items)
	err = r.Get(ctx, req.NamespacedName, mondoo)
	if err != nil {
		log.Error(err, "Failed to get mondoo")
		return ctrl.Result{}, err
	}

	// Update status.Pods list if needed
	statusPodNames := sets.NewString(mondoo.Status.Pods...)

	if !statusPodNames.Equal(podListNames) {
		mondoo.Status.Pods = podListNames.List()
		err := r.Status().Update(ctx, mondoo)
		if err != nil {
			log.Error(err, "Failed to update mondoo status")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{Requeue: true, RequeueAfter: time.Hour * 24 * 7}, nil
}

// labelsForMondoo returns the labels for selecting the resources
// belonging to the given mondoo CR name.
func labelsForMondoo(name string) map[string]string {
	return map[string]string{"app": "mondoo", "mondoo_cr": name}
}

// getPodNames returns a Set of the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) sets.String {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return sets.NewString(podNames...)
}

// SetupWithManager sets up the controller with the Manager.
func (r *MondooAuditConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.MondooAuditConfig{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

func hasFinalizer(mac *v1alpha1.MondooAuditConfig) bool {
	for _, f := range mac.Finalizers {
		if f == finalizerString {
			return true
		}
	}
	return false
}

func addFinalizer(mac *v1alpha1.MondooAuditConfig) {
	finalizerSet := sets.NewString(mac.Finalizers...)
	finalizerSet.Insert(finalizerString)
	mac.SetFinalizers(finalizerSet.List())

	return
}

func removeFinalizer(mac *v1alpha1.MondooAuditConfig) {
	finalizerSet := sets.NewString(mac.Finalizers...)
	finalizerSet.Delete(finalizerString)
	mac.SetFinalizers(finalizerSet.List())

	return
}
