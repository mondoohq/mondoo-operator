/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package controllers

import (
	"context"
	"time"

	"github.com/go-logr/logr"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers/admission"
	"go.mondoo.com/mondoo-operator/controllers/container_image"
	"go.mondoo.com/mondoo-operator/controllers/k8s_scan"
	"go.mondoo.com/mondoo-operator/controllers/nodes"
	"go.mondoo.com/mondoo-operator/controllers/resource_monitor/scan_api_store"
	"go.mondoo.com/mondoo-operator/controllers/scanapi"
	"go.mondoo.com/mondoo-operator/controllers/status"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.mondoo.com/mondoo-operator/pkg/version"
)

const finalizerString = "k8s.mondoo.com/delete"

// MondooAuditConfigReconciler reconciles a MondooAuditConfig object
type MondooAuditConfigReconciler struct {
	client.Client
	MondooClientBuilder    func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error)
	ContainerImageResolver mondoo.ContainerImageResolver
	StatusReporter         *status.StatusReporter
	RunningOnOpenShift     bool
	ScanApiStore           scan_api_store.ScanApiStore
}

// so we can mock out the mondoo client for testing
var MondooClientBuilder = mondooclient.NewClient

// The update permissions for MondooAuditConfigs are required because having update permissions just for finalizers is insufficient
// to add finalizers. There is a github issue describing the problem https://github.com/kubernetes-sigs/kubebuilder/issues/2264
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooauditconfigs,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooauditconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooauditconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondoooperatorconfigs,verbs=get;watch;list
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments;replicasets;daemonsets;statefulsets,verbs=get;list;watch
//+kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=cronjobs;jobs,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods;namespaces;nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// Just neeed to be able to create a Secret to hold the generated ScanAPI token
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=create;delete
// Need to be able to check for the existence of Secrets with tokens, Mondoo service accounts, and private image pull secrets without asking for permission to read all Secrets
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get
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
func (r *MondooAuditConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (reconcileResult ctrl.Result, reconcileError error) {
	log := ctrllog.FromContext(ctx)

	// Fetch the Mondoo instance
	mondooAuditConfig := &v1alpha2.MondooAuditConfig{}

	reconcileError = r.Get(ctx, req.NamespacedName, mondooAuditConfig)

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

	if !mondooAuditConfig.DeletionTimestamp.IsZero() {
		log.Info("deleting")

		// Any other Reconcile() loops that need custom cleanup when the MondooAuditConfig is being
		// deleted should be called here

		webhooks := admission.DeploymentHandler{
			Mondoo:                 mondooAuditConfig,
			KubeClient:             r.Client,
			TargetNamespace:        req.Namespace,
			MondooOperatorConfig:   config,
			ContainerImageResolver: r.ContainerImageResolver,
		}
		result, reconcileError := webhooks.Reconcile(ctx)
		if reconcileError != nil {
			log.Error(reconcileError, "failed to cleanup webhooks")
			return result, reconcileError
		}

		controllerutil.RemoveFinalizer(mondooAuditConfig, finalizerString)
		if reconcileError = r.Update(ctx, mondooAuditConfig); reconcileError != nil {
			log.Error(reconcileError, "failed to remove finalizer")
		}
		return ctrl.Result{}, reconcileError
	} else {
		if !controllerutil.ContainsFinalizer(mondooAuditConfig, finalizerString) {
			controllerutil.AddFinalizer(mondooAuditConfig, finalizerString)
			if reconcileError = r.Update(ctx, mondooAuditConfig); reconcileError != nil {
				log.Error(reconcileError, "failed to set finalizer")
			}
			return ctrl.Result{}, reconcileError
		}
	}

	mondooAuditConfigCopy := mondooAuditConfig.DeepCopy()

	// Conditions might be updated before this reconciler reaches the end
	// MondooAuditConfig has to include these updates in any case.
	defer func() {
		var deferFuncErr error
		ctx := context.Background()
		// Update the mondoo status with the pod names only after all pod creation actions are done
		// List the pods for this mondoo's cronjobs and deployment
		podList := &corev1.PodList{}
		listOpts := []client.ListOption{
			client.InNamespace(mondooAuditConfig.Namespace),
			client.MatchingLabels(labelsForMondoo(mondooAuditConfig.Name)),
		}
		if deferFuncErr = r.List(ctx, podList, listOpts...); deferFuncErr != nil {
			log.Error(deferFuncErr, "Failed to list pods", "Mondoo.Namespace", mondooAuditConfig.Namespace, "Mondoo.Name", mondooAuditConfig.Name)
		} else {
			podListNames := getPodNames(podList.Items)

			// Update status.Pods list if needed
			statusPodNames := sets.New(mondooAuditConfig.Status.Pods...)

			if !statusPodNames.Equal(podListNames) {
				mondooAuditConfig.Status.Pods = podListNames.UnsortedList()
			}
		}

		deferFuncErr = mondoo.UpdateMondooAuditStatus(ctx, r.Client, mondooAuditConfigCopy, mondooAuditConfig, log)
		// do not overwrite errors which happened before the defered function is called
		// but in case an error happend in the defered func, also overwrite the reconcileResult
		if reconcileError == nil && deferFuncErr != nil {
			reconcileResult = ctrl.Result{}
			reconcileError = deferFuncErr
		}
	}()

	// If spec.MondooTokenSecretRef != "" and the Secret referenced in spec.MondooCredsSecretRef
	// does not exist, then attempt to trade the token for a Mondoo service account and save it
	// in the Secret referenced in .spec.MondooCredsSecretRef
	if reconcileError = r.exchangeTokenForServiceAccount(ctx, mondooAuditConfig, log); reconcileError != nil {
		log.Error(reconcileError, "errors while checking if Mondoo service account needs creating")
		return ctrl.Result{}, reconcileError
	}

	scanapi := scanapi.DeploymentHandler{
		Mondoo:                 mondooAuditConfig,
		KubeClient:             r.Client,
		MondooOperatorConfig:   config,
		ContainerImageResolver: r.ContainerImageResolver,
		DeployOnOpenShift:      r.RunningOnOpenShift,
	}
	result, reconcileError := scanapi.Reconcile(ctx)
	if reconcileError != nil {
		log.Error(reconcileError, "Failed to set up scan API")
	}
	if reconcileError != nil || result.Requeue {
		return result, reconcileError
	}

	nodes := nodes.DeploymentHandler{
		Mondoo:                 mondooAuditConfig,
		KubeClient:             r.Client,
		MondooOperatorConfig:   config,
		ContainerImageResolver: r.ContainerImageResolver,
		IsOpenshift:            r.RunningOnOpenShift,
	}

	result, reconcileError = nodes.Reconcile(ctx)
	if reconcileError != nil {
		log.Error(reconcileError, "Failed to set up nodes scanning")
	}
	if reconcileError != nil || result.Requeue {
		return result, reconcileError
	}

	containers := container_image.DeploymentHandler{
		Mondoo:                 mondooAuditConfig,
		KubeClient:             r.Client,
		ContainerImageResolver: r.ContainerImageResolver,
		MondooOperatorConfig:   config,
	}

	result, reconcileError = containers.Reconcile(ctx)
	if reconcileError != nil {
		log.Error(reconcileError, "Failed to set up container scanning")
	}
	if reconcileError != nil || result.Requeue {
		return result, reconcileError
	}

	workloads := k8s_scan.DeploymentHandler{
		Mondoo:                 mondooAuditConfig,
		KubeClient:             r.Client,
		MondooOperatorConfig:   config,
		ContainerImageResolver: r.ContainerImageResolver,
		ScanApiStore:           r.ScanApiStore,
	}

	result, reconcileError = workloads.Reconcile(ctx)
	if reconcileError != nil {
		log.Error(reconcileError, "Failed to set up Kubernetes resources scanning")
	}
	if reconcileError != nil || result.Requeue {
		return result, reconcileError
	}

	webhooks := admission.DeploymentHandler{
		Mondoo:                 mondooAuditConfig,
		KubeClient:             r.Client,
		TargetNamespace:        req.Namespace,
		MondooOperatorConfig:   config,
		ContainerImageResolver: r.ContainerImageResolver,
	}

	result, reconcileError = webhooks.Reconcile(ctx)
	if reconcileError != nil {
		log.Error(reconcileError, "Failed to set up webhooks")
	}
	if reconcileError != nil || result.Requeue {
		return result, reconcileError
	}

	// Update status.ReconciledByOperatorVersion to the running operator version
	// This should only happen, after all objects have been reconciled
	mondooAuditConfig.Status.ReconciledByOperatorVersion = version.Version

	return ctrl.Result{Requeue: true, RequeueAfter: time.Hour * 24 * 7}, nil
}

// nodeEventsRequestMapper Maps node events to enqueue all MondooAuditConfigs that have node scanning enabled for
// reconciliation.
func (r *MondooAuditConfigReconciler) nodeEventsRequestMapper(o client.Object) []reconcile.Request {
	ctx := context.Background()
	var requests []reconcile.Request
	auditConfigs := &v1alpha2.MondooAuditConfigList{}
	if err := r.Client.List(ctx, auditConfigs); err != nil {
		logger := ctrllog.Log.WithName("node-watcher")
		logger.Error(err, "Failed to list MondooAuditConfigs")
		return requests
	}

	for _, a := range auditConfigs.Items {
		// Only enqueue the MondooAuditConfig if it has node scanning enabled.
		if a.Spec.Nodes.Enable {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&a)})
		}
	}
	return requests
}

func (r *MondooAuditConfigReconciler) exchangeTokenForServiceAccount(ctx context.Context, auditConfig *v1alpha2.MondooAuditConfig, log logr.Logger) error {
	if auditConfig.Spec.MondooCredsSecretRef.Name == "" {
		log.Info("MondooAuditConfig without .spec.mondooCredsSecretRef defined")
		return nil
	}

	mondooCredsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      auditConfig.Spec.MondooCredsSecretRef.Name,
			Namespace: auditConfig.Namespace,
		},
	}
	mondooCredsExists, err := k8s.CheckIfExists(ctx, r.Client, mondooCredsSecret, mondooCredsSecret)
	if err != nil {
		log.Error(err, "failed to check whether Mondoo creds secret exists")
		return err
	}

	if mondooCredsExists {
		// Nothing to do as we already have creds
		return nil
	}

	mondooTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      auditConfig.Spec.MondooTokenSecretRef.Name,
			Namespace: auditConfig.Namespace,
		},
	}
	mondooTokenExists, err := k8s.CheckIfExists(ctx, r.Client, mondooTokenSecret, mondooTokenSecret)
	if err != nil {
		log.Error(err, "failed to check whether Mondoo token secret exists")
		return err
	}

	// mondoCredsExists is already false from here down
	if !mondooTokenExists {
		log.Info("neither .spec.MondooCredsSecretRef nor .spec.MondooTokenSecretRef exist")
		return nil
	}

	log.Info("Creating Mondoo service account from token")
	tokenData := string(mondooTokenSecret.Data[constants.MondooTokenSecretKey])
	return mondoo.CreateServiceAccountFromToken(
		ctx,
		r.Client,
		r.MondooClientBuilder,
		auditConfig.Spec.ConsoleIntegration.Enable,
		client.ObjectKeyFromObject(mondooCredsSecret),
		tokenData,
		auditConfig.Spec.HttpProxy,
		log)
}

// SetupWithManager sets up the controller with the Manager.
func (r *MondooAuditConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.MondooAuditConfig{}).
		Owns(&batchv1.CronJob{}).
		Owns(&appsv1.Deployment{}).
		Watches(
			&source.Kind{Type: &corev1.Node{}},
			handler.EnqueueRequestsFromMapFunc(r.nodeEventsRequestMapper),
			builder.WithPredicates(k8s.IgnoreGenericEventsPredicate{})).
		Complete(r)
}

// labelsForMondoo returns the labels for selecting the resources
// belonging to the given mondoo CR name.
func labelsForMondoo(name string) map[string]string {
	return map[string]string{"mondoo_cr": name}
}

// getPodNames returns a Set of the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) sets.Set[string] {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return sets.New(podNames...)
}
