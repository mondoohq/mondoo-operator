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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	jwt "github.com/golang-jwt/jwt/v4"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	"go.mondoo.com/mondoo-operator/controllers/integration"
	"go.mondoo.com/mondoo-operator/controllers/k8s_scan"
	"go.mondoo.com/mondoo-operator/controllers/nodes"
	"go.mondoo.com/mondoo-operator/controllers/scanapi"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

const finalizerString = "k8s.mondoo.com/delete"

// MondooAuditConfigReconciler reconciles a MondooAuditConfig object
type MondooAuditConfigReconciler struct {
	client.Client
	Scheme                 *runtime.Scheme
	MondooClientBuilder    func(mondooclient.ClientOptions) mondooclient.Client
	ContainerImageResolver mondoo.ContainerImageResolver
}

// Embed the Default Inventory for CronJob and Deployment Configurations
var (
	// so we can mock out the mondoo client for testing
	MondooClientBuilder = mondooclient.NewClient
)

// The update permissions for MondooAuditConfigs are required because having update permissions just for finalizers is insufficient
// to add finalizers. There is a github issue describing the problem https://github.com/kubernetes-sigs/kubebuilder/issues/2264
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooauditconfigs,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooauditconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooauditconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondoooperatorconfigs,verbs=get;watch;list
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods;namespaces;nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// Just neeed to be able to create a Secret to hold the generated ScanAPI token
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=create;delete
// Need to be able to check for the existence of Secrets with tokens and Mondoo service accounts without asking for permission to read all Secrets
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get,resourceNames=mondoo-client;mondoo-token
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
	mondooAuditConfig := &v1alpha2.MondooAuditConfig{}

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
	config := &v1alpha2.MondooOperatorConfig{}
	if err := r.Get(ctx, types.NamespacedName{Name: v1alpha2.MondooOperatorConfigName}, config); err != nil {
		if errors.IsNotFound(err) {
			log.Info("MondooOperatorConfig not found, using defaults")
		} else {
			log.Error(err, "Failed to check for MondooOpertorConfig")
			return ctrl.Result{}, err
		}
	}

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

	// TODO: extract deployment of scan API here instead of having it separate for admission and k8s_scan as this does not work

	mondooAuditConfigCopy := mondooAuditConfig.DeepCopy()

	// If spec.MondooTokenSecretRef != "" and the Secret referenced in spec.MondooCredsSecretRef
	// does not exist, then attempt to trade the token for a Mondoo service account and save it
	// in the Secret referenced in .spec.MondooCredsSecretRef
	if err := r.exchangeTokenForServiceAccount(ctx, mondooAuditConfig, log); err != nil {
		log.Error(err, "errors while checking if Mondoo service account needs creating")
		return ctrl.Result{}, err
	}

	scanapi := scanapi.DeploymentHandler{
		Mondoo:                 mondooAuditConfig,
		KubeClient:             r.Client,
		MondooOperatorConfig:   config,
		ContainerImageResolver: r.ContainerImageResolver,
	}
	result, err := scanapi.Reconcile(ctx)
	if err != nil {
		log.Error(err, "Failed to set up scan API")
	}
	if err != nil || result.Requeue {
		return result, err
	}

	nodes := nodes.DeploymentHandler{
		Mondoo:                 mondooAuditConfig,
		KubeClient:             r.Client,
		MondooOperatorConfig:   config,
		ContainerImageResolver: r.ContainerImageResolver,
	}

	result, err = nodes.Reconcile(ctx)
	if err != nil {
		log.Error(err, "Failed to set up nodes scanning")
	}
	if err != nil || result.Requeue {
		return result, err
	}

	workloads := k8s_scan.DeploymentHandler{
		Mondoo:                 mondooAuditConfig,
		KubeClient:             r.Client,
		MondooOperatorConfig:   config,
		ContainerImageResolver: r.ContainerImageResolver,
	}

	result, err = workloads.Reconcile(ctx)
	if err != nil {
		log.Error(err, "Failed to set up Kubernetes resources scanning")
	}
	if err != nil || result.Requeue {
		return result, err
	}

	webhooks := admission.DeploymentHandler{
		Mondoo:                 mondooAuditConfig,
		KubeClient:             r.Client,
		TargetNamespace:        req.Namespace,
		MondooOperatorConfig:   config,
		ContainerImageResolver: r.ContainerImageResolver,
	}

	result, err = webhooks.Reconcile(ctx)
	if err != nil {
		log.Error(err, "Failed to set up webhooks")
	}
	if err != nil || result.Requeue {
		return result, err
	}

	// Update the mondoo status with the pod names only after all pod creation actions are done
	// List the pods for this mondoo's cronjobs and deployment
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

func (r *MondooAuditConfigReconciler) exchangeTokenForServiceAccount(ctx context.Context, mondoo *v1alpha2.MondooAuditConfig, log logr.Logger) error {
	if mondoo.Spec.MondooCredsSecretRef.Name == "" {
		log.Info("MondooAuditConfig without .spec.mondooCredsSecretRef defined")
		return nil
	}

	mondooCredsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mondoo.Spec.MondooCredsSecretRef.Name,
			Namespace: mondoo.Namespace,
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
			Name:      mondoo.Spec.MondooTokenSecretRef.Name,
			Namespace: mondoo.Namespace,
		},
	}
	mondooTokenExists, err := k8s.CheckIfExists(ctx, r.Client, mondooTokenSecret, mondooTokenSecret)
	if err != nil {
		log.Error(err, "failed to cehck whether Mondoo token secret exists")
		return err
	}

	// mondoCredsExists is already false from here down
	if !mondooTokenExists {
		log.Info("neither .spec.MondooCredsSecretRef nor .spec.MondooTokenSecretRef exist")
		return nil
	}

	log.Info("Creating Mondoo service account from token")
	token := strings.TrimSpace(string(mondooTokenSecret.Data[constants.MondooTokenSecretKey]))

	return r.createServiceAccountFromToken(ctx, mondoo, token, log)

}

func (r *MondooAuditConfigReconciler) createServiceAccountFromToken(ctx context.Context, mondoo *v1alpha2.MondooAuditConfig, jwtString string, log logr.Logger) error {

	parser := &jwt.Parser{}
	token, _, err := parser.ParseUnverified(jwtString, jwt.MapClaims{})
	if err != nil {
		log.Error(err, "failed to parse token")
		return err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		err := fmt.Errorf("failed to type asesrt claims from token")
		log.Error(err, "failed to extract claim")
		return err
	}
	apiEndpoint := claims["api_endpoint"]

	opts := mondooclient.ClientOptions{
		ApiEndpoint: fmt.Sprintf("%v", apiEndpoint),
		Token:       jwtString,
	}

	mClient := r.MondooClientBuilder(opts)

	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mondoo.Spec.MondooCredsSecretRef.Name,
			Namespace: mondoo.Namespace,
		},
	}
	if mondoo.Spec.ConsoleIntegration.Enable {
		// owner is the MRN of the integration
		tokenOwner, ok := claims["owner"]
		if !ok {
			err := fmt.Errorf("'owner' claim missing from token which is expected for Mondoo integration registration")
			log.Error(err, "missing data in token Secret")
			return err
		}
		// Do an integration-style registration to associate the generated
		// service account with the Mondoo console Integration
		resp, err := mClient.IntegrationRegister(ctx, &mondooclient.IntegrationRegisterInput{
			Mrn:   fmt.Sprintf("%v", tokenOwner),
			Token: jwtString,
		})
		if err != nil {
			log.Error(err, "failed to exchange token for a service account")
			return err
		}

		integrationMrn := resp.Mrn
		credsBytes, err := json.Marshal(*resp.Creds)
		if err != nil {
			log.Error(err, "failed to marshal service account creds from IntegrationRegister()")
			return err
		}
		tokenSecret.StringData = map[string]string{
			constants.MondooCredsSecretServiceAccountKey: string(credsBytes),
			constants.MondooCredsSecretIntegrationMRNKey: integrationMrn,
		}
		_, err = k8s.CreateIfNotExist(ctx, r.Client, tokenSecret, tokenSecret)
		if err != nil {
			log.Error(err, "error while trying to save Mondoo service account into secret")
			return err
		}

		// No easy way to retry this one-off CheckIn(). An error on initial CheckIn()
		// means we'll just retry on the regularly scheduled interval via the integration controller
		_ = r.performInitialCheckIn(integrationMrn, credsBytes, log)
	} else {
		// Do a vanilla token-for-service-account exchange
		resp, err := mClient.ExchangeRegistrationToken(ctx, &mondooclient.ExchangeRegistrationTokenInput{
			Token: jwtString,
		})
		if err != nil {
			log.Error(err, "failed to exchange token for a service account")
			return err
		}

		// Save the service account
		tokenSecret.StringData = map[string]string{
			constants.MondooCredsSecretServiceAccountKey: resp.ServiceAccount,
		}
		_, err = k8s.CreateIfNotExist(ctx, r.Client, tokenSecret, tokenSecret)
		if err != nil {
			log.Error(err, "error while trying to save Mondoo service account into secret")
			return err
		}
	}

	log.Info("saved Mondoo service account", "secret", fmt.Sprintf("%s/%s", mondoo.Namespace, mondoo.Spec.MondooCredsSecretRef.Name))

	return nil
}

func (r *MondooAuditConfigReconciler) performInitialCheckIn(integrationMrn string, serviceAccount []byte, logger logr.Logger) error {

	// build a minimal IntegrationReconciler to be able to attempt a CheckIn()
	integrationReconciler := &integration.IntegrationReconciler{
		Log:                 logger,
		MondooClientBuilder: r.MondooClientBuilder,
	}

	if err := integrationReconciler.IntegrationCheckIn([]byte(integrationMrn), serviceAccount); err != nil {
		logger.Error(err, "initial CheckIn() failed, will CheckIn() periodically", "integrationMRN", integrationMrn)
		return err
	}
	return nil
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
func getPodNames(pods []corev1.Pod) sets.String {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return sets.NewString(podNames...)
}
