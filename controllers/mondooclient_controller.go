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
	"reflect"

	"go.mondoo.com/mondoo-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// MondooClientReconciler reconciles a MondooClient object
type MondooClientReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooclients,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooclients/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooclients/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MondooClient object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *MondooClientReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	// Fetch the Mondoo instance
	mondoo := &v1alpha1.MondooClient{}
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

	// Check if the Secret already exists, if not create a new one
	foundSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: mondoo.Name, Namespace: mondoo.Namespace}, foundSecret)
	if err != nil && errors.IsNotFound(err) {
		// Define a new secret
		dep := r.secretForMondoo(mondoo)
		log.Info("Creating a new secret ", "Secret.Namespace", dep.Namespace, "Secret.Name", dep.Name)
		err = r.Create(ctx, dep)
		if err != nil {
			log.Error(err, "Failed to create new Secret", "Secret.Namespace", dep.Namespace, "Secret.Name", dep.Name)
			return ctrl.Result{}, err
		}
		// secret created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Secret")
		return ctrl.Result{}, err
	}
	// Check if the Secret already exists, if not create a new one
	foundConfigMap := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: mondoo.Name, Namespace: mondoo.Namespace}, foundConfigMap)
	if err != nil && errors.IsNotFound(err) {
		// Define a new secret
		dep := r.configMapForMondoo(mondoo)
		log.Info("Creating a new configmap", "ConfigMap.Namespace", dep.Namespace, "ConfigMap.Name", dep.Name)
		err = r.Create(ctx, dep)
		if err != nil {
			log.Error(err, "Failed to create new Secret", "ConfigMap.Namespace", dep.Namespace, "ConfigMap.Name", dep.Name)
			return ctrl.Result{}, err
		}
		// secret created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Secret")
		return ctrl.Result{}, err
	}

	// Check if the daemonset already exists, if not create a new one
	found := &appsv1.DaemonSet{}
	err = r.Get(ctx, types.NamespacedName{Name: mondoo.Name, Namespace: mondoo.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Define a new daemonset
		dep := r.deamonsetForMondoo(mondoo)
		log.Info("Creating a new Daemonset", "Daemonset.Namespace", dep.Namespace, "Daemonset.Name", dep.Name)
		err = r.Create(ctx, dep)
		if err != nil {
			log.Error(err, "Failed to create new Daemonset", "Daemonset.Namespace", dep.Namespace, "Daemonset.Name", dep.Name)
			return ctrl.Result{}, err
		}
		// Daemonset created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Daemonset")
		return ctrl.Result{}, err
	}

	// Update the mondoo status with the pod names
	// List the pods for this mondoo's daemonset
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

// deamonsetForMondoo returns a mondoo Daemonset object
func (r *MondooClientReconciler) deamonsetForMondoo(m *v1alpha1.MondooClient) *appsv1.DaemonSet {
	ls := labelsForMondoo(m.Name)

	dep := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Tolerations: []corev1.Toleration{{
						Key:    "node-role.kubernetes.io/master",
						Effect: corev1.TaintEffect("NoSchedule"),
					}},
					Containers: []corev1.Container{{
						Image:   "mondoolabs/mondoo:latest",
						Name:    "mondoo-agent",
						Command: []string{"mondoo", "serve", "--config", "/etc/opt/mondoo/mondoo.yml"},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "root",
								ReadOnly:  true,
								MountPath: "/mnt/host/",
							},
							{
								Name:      "config",
								ReadOnly:  true,
								MountPath: "/etc/opt/",
							},
						},

						Env: []corev1.EnvVar{
							{
								Name:  "DEBUG",
								Value: "false",
							},
							{
								Name:  "MONDOO_PROCFS",
								Value: "on",
							},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: "root",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/",
								},
							},
						},
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: []corev1.VolumeProjection{
										{
											ConfigMap: &corev1.ConfigMapProjection{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: m.Name,
												},
												Items: []corev1.KeyToPath{{
													Key:  "inventory",
													Path: "mondoo/inventory.yml",
												}},
											},
										},
										{
											Secret: &corev1.SecretProjection{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: m.Name,
												},
												Items: []corev1.KeyToPath{{
													Key:  "config",
													Path: "mondoo/mondoo.yml",
												}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	// Set mondoo instance as the owner and controller
	ctrl.SetControllerReference(m, dep, r.Scheme)
	return dep
}

func (r *MondooClientReconciler) secretForMondoo(m *v1alpha1.MondooClient) *corev1.Secret {

	dep := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Data: map[string][]byte{
			"config": []byte(m.Data.Config),
		},
	}
	// Set mondoo instance as the owner and controller
	ctrl.SetControllerReference(m, dep, r.Scheme)

	return dep
}

func (r *MondooClientReconciler) configMapForMondoo(m *v1alpha1.MondooClient) *corev1.ConfigMap {

	dep := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Data: map[string]string{
			"inventory": m.Data.Inventory,
		},
	}
	// Set mondoo instance as the owner and controller
	ctrl.SetControllerReference(m, dep, r.Scheme)

	return dep
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
func (r *MondooClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.MondooClient{}).
		Owns(&appsv1.DaemonSet{}).
		Complete(r)
}
