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
	"go.mondoo.com/mondoo-operator/controllers/webhooks"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
var (
	//go:embed inventory-ds.yaml
	dsInventoryyaml []byte

	//go:embed inventory-deploy.yaml
	deployInventoryyaml []byte

	// Defined as a var so we can easily mock this in tests.
	createContainerImageResolver = mondoo.NewContainerImageResolver
)

// The update permissions for MondooAuditConfigs are required because having update permissions just for finalizers is insufficient
// to add finalizers. There is a github issue describing the problem https://github.com/kubernetes-sigs/kubebuilder/issues/2264
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooauditconfigs,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooauditconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooauditconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondoooperatorconfigs,verbs=get;watch;list
//+kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods;namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// Just neeed to be able to create a Secret to hold the generated ScanAPI token
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=create;delete
//+kubebuilder:rbac:groups=cert-manager.io,resources=certificates;issuers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
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
	mondooAuditConfig := &v1alpha1.MondooAuditConfig{}

	err := r.Get(ctx, req.NamespacedName, mondooAuditConfig)

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
	config := &v1alpha1.MondooOperatorConfig{}
	if err := r.Get(ctx, types.NamespacedName{Name: v1alpha1.MondooOperatorConfigName}, config); err != nil {
		if errors.IsNotFound(err) {
			log.Info("MondooOperatorConfig not found, using defaults")
		} else {
			log.Error(err, "Failed to check for MondooOpertorConfig")
			return ctrl.Result{}, err
		}
	}

	// Have on container image resolver for the whole reconcile cycle. This will make sure that the same image is
	// not resolved multiple times during a single reconcile.
	containerImageResolver := createContainerImageResolver()

	if config.DeletionTimestamp != nil {
		// Going to proceed as if there is no MondooOperatorConfig
		config = &v1alpha1.MondooOperatorConfig{}
	}

	if mondooAuditConfig.DeletionTimestamp != nil {
		log.Info("deleting")

		// Any other Reconcile() loops that need custom cleanup when the MondooAuditConfig is being
		// deleted should be called here

		webhooks := webhooks.Webhooks{
			Mondoo:                 mondooAuditConfig,
			KubeClient:             r.Client,
			TargetNamespace:        req.Namespace,
			MondooOperatorConfig:   config,
			ContainerImageResolver: containerImageResolver,
		}
		result, err := webhooks.Reconcile(ctx)
		if err != nil {
			log.Error(err, "failed to cleanup webhooks")
			return result, err
		}

		controllerutil.RemoveFinalizer(mondooAuditConfig, finalizerString)
		if err := r.Update(ctx, mondooAuditConfig); err != nil {
			log.Error(err, "failed to remove finalizer")
		}
		return ctrl.Result{}, err
	} else {
		if !controllerutil.ContainsFinalizer(mondooAuditConfig, finalizerString) {
			controllerutil.AddFinalizer(mondooAuditConfig, finalizerString)
			if err := r.Update(ctx, mondooAuditConfig); err != nil {
				log.Error(err, "failed to set finalizer")
			}
			return ctrl.Result{}, err
		}
	}

	mondooAuditConfigCopy := mondooAuditConfig.DeepCopy()
	nodes := Nodes{
		Enable:                 mondooAuditConfig.Spec.Nodes.Enable,
		Mondoo:                 mondooAuditConfig,
		MondooOperatorConfig:   config,
		ContainerImageResolver: containerImageResolver,
	}

	result, err := nodes.Reconcile(ctx, r.Client, r.Scheme, req, string(dsInventoryyaml))
	if err != nil {
		log.Error(err, "Failed to declare nodes")
	}
	if err != nil || result.Requeue {
		return result, err
	}

	workloads := Workloads{
		Enable:                 mondooAuditConfig.Spec.Workloads.Enable,
		Mondoo:                 mondooAuditConfig,
		MondooOperatorConfig:   config,
		ContainerImageResolver: containerImageResolver,
	}

	result, err = workloads.Reconcile(ctx, r.Client, r.Scheme, req, string(deployInventoryyaml))
	if err != nil {
		log.Error(err, "Failed to declare workloads")
	}
	if err != nil || result.Requeue {
		return result, err
	}

	webhooks := webhooks.Webhooks{
		Mondoo:                 mondooAuditConfig,
		KubeClient:             r.Client,
		TargetNamespace:        req.Namespace,
		MondooOperatorConfig:   config,
		ContainerImageResolver: containerImageResolver,
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
		client.InNamespace(mondooAuditConfig.Namespace),
		client.MatchingLabels(labelsForMondoo(mondooAuditConfig.Name)),
	}
	if err = r.List(ctx, podList, listOpts...); err != nil {
		log.Error(err, "Failed to list pods", "Mondoo.Namespace", mondooAuditConfig.Namespace, "Mondoo.Name", mondooAuditConfig.Name)
		return ctrl.Result{}, err
	}
	podListNames := getPodNames(podList.Items)

	// Update status.Pods list if needed
	statusPodNames := sets.NewString(mondooAuditConfig.Status.Pods...)

	if !statusPodNames.Equal(podListNames) {
		mondooAuditConfig.Status.Pods = podListNames.List()
		err := r.Status().Update(ctx, mondooAuditConfig)
		if err != nil {
			log.Error(err, "Failed to update mondoo status")
			return ctrl.Result{}, err
		}
	}

	if err := mondoo.UpdateMondooAuditStatus(ctx, r.Client, mondooAuditConfigCopy, mondooAuditConfig, log); err != nil {
		return ctrl.Result{}, err
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
