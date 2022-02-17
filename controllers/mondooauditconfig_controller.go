/*
Copyright 2022.

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
	"reflect"

	"go.mondoo.com/mondoo-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// MondooAuditConfigReconciler reconciles a MondooAuditConfig object
type MondooAuditConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Interface for workloads that mondooAuditConfig CRD creates
type Mondoo interface {
	Reconcile(ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request, inventory string) (ctrl.Result, error)
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
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
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
			log.Info("mondoo resource not found. Ignoring since object must be deleted")
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, client.IgnoreNotFound(err)

		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get mondoo")
		return ctrl.Result{}, err
	}

	myFinalizerName := "batch.tutorial.kubebuilder.io/finalizer"

	// examine DeletionTimestamp to determine if object is under deletion
	if mondoo.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(mondoo, myFinalizerName) {
			controllerutil.AddFinalizer(mondoo, myFinalizerName)
			if err := r.Update(ctx, mondoo); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(mondoo, myFinalizerName) {
			// our finalizer is present, so lets handle any external dependency

			mondoo.Spec.Nodes.Enable = false
			mondoo.Spec.Workloads.Enable = false
			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(mondoo, myFinalizerName)
			if err := r.Update(ctx, mondoo); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	nodes := Nodes{
		Enable: mondoo.Spec.Nodes.Enable,
		Mondoo: *mondoo,
	}

	workloads := Workloads{
		Enable: mondoo.Spec.Workloads.Enable,
		Mondoo: *mondoo,
	}

	Up(&nodes, ctx, r.Client, r.Scheme, req, string(dsInventoryyaml))
	Up(&workloads, ctx, r.Client, r.Scheme, req, string(deployInventoryyaml))

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
	podNames := getPodNames(podList.Items)
	err = r.Get(ctx, req.NamespacedName, mondoo)
	if err != nil {
		log.Error(err, "Failed to get mondoo")
		return ctrl.Result{}, err
	}
	// Update status.Nodes if needed
	if !reflect.DeepEqual(podNames, mondoo.Status.Nodes) {
		mondoo.Status.Nodes = podNames
		err := r.Status().Update(ctx, mondoo)
		if err != nil {
			log.Error(err, "Failed to update mondoo status")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func Up(g Mondoo, ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request, inventory string) {
	g.Reconcile(ctx, clt, scheme, req, inventory)
}

// labelsForMondoo returns the labels for selecting the resources
// belonging to the given mondoo CR name.
func labelsForMondoo(name string) map[string]string {
	return map[string]string{"app": "mondoo", "mondoo_cr": name}
}

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

// SetupWithManager sets up the controller with the Manager.
func (r *MondooAuditConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.MondooAuditConfig{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
