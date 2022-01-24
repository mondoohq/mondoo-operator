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
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// MondooClientReconciler reconciles a MondooClient object
type MondooClientReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Embed the Default Inventory for Daemonset and Deployment Configurations
//go:embed inventory-ds.yaml
var dsInventoryyaml []byte

//go:embed inventory-deploy.yaml
var deployInventoryyaml []byte

type Inventory struct {
	Inventory string `yaml:"inventory,flow"`
}

var dsInventory Inventory
var deployInventory Inventory

//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooclients,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooclients/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.mondoo.com,resources=mondooclients/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
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
// the MondooClient object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *MondooClientReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	err := yaml.Unmarshal(dsInventoryyaml, &dsInventory)
	if err != nil {
		log.Error(err, "could not load default inventory for daemonset")
	}
	err = yaml.Unmarshal(deployInventoryyaml, &deployInventory)
	if err != nil {
		log.Error(err, "could not load default inventory for deployment")
	}
	// Fetch the Mondoo instance
	mondoo := &v1alpha1.MondooClient{}

	err = r.Get(ctx, req.NamespacedName, mondoo)
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
	inventoryDaemonSet := mondoo.Name + "-ds"
	inventoryDeployment := mondoo.Name + "-deploy"
	// Check if the Credential Secret already exists, if not create a new one
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

	if !mondoo.Data.KubeNodes.Disabled {

		// Check if the Inventory Config already exists, if not create a new one
		foundConfigMap := &corev1.ConfigMap{}
		err = r.Get(ctx, types.NamespacedName{Name: inventoryDaemonSet, Namespace: mondoo.Namespace}, foundConfigMap)
		if err != nil && errors.IsNotFound(err) {
			// Define a new configmap
			dep := r.configMapForMondooDaemonSet(mondoo, inventoryDaemonSet, dsInventory.Inventory)
			log.Info("Creating a new configmap", "ConfigMap.Namespace", dep.Namespace, "ConfigMap.Name", inventoryDaemonSet)

			err = r.Create(ctx, dep)
			if err != nil {
				log.Error(err, "Failed to create new Configmap", "ConfigMap.Namespace", dep.Namespace, "ConfigMap.Name", inventoryDaemonSet)
				return ctrl.Result{}, err
			}
			// configmap created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			log.Error(err, "Failed to get Configmap")
			return ctrl.Result{}, err
		}

		// Check if the daemonset already exists, if not create a new one
		found := &appsv1.DaemonSet{}
		err = r.Get(ctx, types.NamespacedName{Name: mondoo.Name, Namespace: mondoo.Namespace}, found)
		if err != nil && errors.IsNotFound(err) {
			// Define a new daemonset
			dep := r.deamonsetForMondoo(mondoo, inventoryDaemonSet)
			log.Info("Creating a new Daemonset", "Daemonset.Namespace", dep.Namespace, "Daemonset.Name", dep.Name)
			err = r.Create(ctx, dep)
			if err != nil {
				log.Error(err, "Failed to create new Daemonset", "Daemonset.Namespace", dep.Namespace, "Daemonset.Name", dep.Name)
				return ctrl.Result{}, err
			}
			// Daemonset created successfully - return and requeue
			return ctrl.Result{Requeue: false}, nil
		} else if err != nil {
			log.Error(err, "Failed to get Daemonset")
			return ctrl.Result{}, err
		}

	}
	if !mondoo.Data.Kubeapi.Disabled {
		// Check if the Inventory Config already exists, if not create a new one
		foundConfigMap := &corev1.ConfigMap{}
		err = r.Get(ctx, types.NamespacedName{Name: inventoryDeployment, Namespace: mondoo.Namespace}, foundConfigMap)
		if err != nil && errors.IsNotFound(err) {
			// Define a new secret
			dep := r.configMapForMondooDeployment(mondoo, inventoryDeployment, deployInventory.Inventory)
			log.Info("Creating a new configmap", "ConfigMap.Namespace", dep.Namespace, "ConfigMap.Name", inventoryDeployment)

			err = r.Create(ctx, dep)
			if err != nil {
				log.Error(err, "Failed to create new Configmap", "ConfigMap.Namespace", dep.Namespace, "ConfigMap.Name", dep.Name)
				return ctrl.Result{}, err
			}
			// configmap created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			log.Error(err, "Failed to get Configmap")
			return ctrl.Result{}, err
		}
		// Check if the Mondoo Service Account already exists, if not create a new one
		foundServiceAccount := &corev1.ServiceAccount{}
		err = r.Get(ctx, types.NamespacedName{Name: mondoo.Name, Namespace: mondoo.Namespace}, foundServiceAccount)
		if err != nil && errors.IsNotFound(err) {
			// Define a new sericeaccount
			dep := r.serviceAccountForMondoo(mondoo)
			log.Info("Creating a new configmap", "ServiceAccount.Namespace", dep.Namespace, "ServiceAccount.Name", dep.Name)

			err = r.Create(ctx, dep)
			if err != nil {
				log.Error(err, "Failed to create new ServiceAccount", "ServiceAccount.Namespace", dep.Namespace, "ServiceAccount.Name", dep.Name)
				return ctrl.Result{}, err
			}
			// configmap created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			log.Error(err, "Failed to get ServiceAccount")
			return ctrl.Result{}, err
		}
		// Check if the Mondoo ClusterRole already exists, if not create a new one
		foundClusterRole := &rbacv1.ClusterRole{}
		err = r.Get(ctx, types.NamespacedName{Name: mondoo.Name, Namespace: ""}, foundClusterRole)
		if err != nil && errors.IsNotFound(err) {
			// Define a new clusterrole
			dep := r.clusterRoleForMondoo(mondoo)
			log.Info("Creating a new clusterRole", "ClusterRole.Name", dep.Name)

			err = r.Create(ctx, dep)
			if err != nil {
				log.Error(err, "Failed to create new ClusterRole", "ClusterRole.Name", dep.Name)
				return ctrl.Result{}, err
			}
			// clusterRole created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			log.Error(err, "Failed to get ClusterRole")
			return ctrl.Result{}, err
		}
		// Check if the Mondoo ClusterRoleBinding already exists, if not create a new one
		foundClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
		err = r.Get(ctx, types.NamespacedName{Name: mondoo.Name, Namespace: ""}, foundClusterRoleBinding)
		if err != nil && errors.IsNotFound(err) {
			// Define a new clusterrolebinding
			dep := r.clusterRoleBindingForMondoo(mondoo)
			log.Info("Creating a new clusterRoleBinding", "ClusterRoleBinding.Name", dep.Name)

			err = r.Create(ctx, dep)
			if err != nil {
				log.Error(err, "Failed to create new ClusterRoleBinding", "ClusterRoleBinding.Name", dep.Name)
				return ctrl.Result{}, err
			}
			// clusterRoleBinding created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			log.Error(err, "Failed to get ClusterRoleBinding")
			return ctrl.Result{}, err
		}
		// Check if the Deployment already exists, if not create a new one
		found := &appsv1.Deployment{}
		err = r.Get(ctx, types.NamespacedName{Name: mondoo.Name, Namespace: mondoo.Namespace}, found)
		if err != nil && errors.IsNotFound(err) {
			// Define a new Deployment
			dep := r.deploymentForMondoo(mondoo, inventoryDeployment)
			log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			err = r.Create(ctx, dep)
			if err != nil {
				log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
				return ctrl.Result{}, err
			}
			// Deployment created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			log.Error(err, "Failed to get Deployment")
			return ctrl.Result{}, err
		}
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

// deamonsetForMondoo returns a  Daemonset object
func (r *MondooClientReconciler) deamonsetForMondoo(m *v1alpha1.MondooClient, cmName string) *appsv1.DaemonSet {
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
													Name: cmName,
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

// deploymentForMondoo returns a Deployment object
func (r *MondooClientReconciler) deploymentForMondoo(m *v1alpha1.MondooClient, cmName string) *appsv1.Deployment {
	ls := labelsForMondoo(m.Name)
	var replicas int32
	if m.Data.Kubeapi.Replicas == 0 {
		replicas = 1
	} else {
		replicas = m.Data.Kubeapi.Replicas
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Replicas: &replicas,
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
					ServiceAccountName: m.Name,
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
													Name: cmName,
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
			"config": []byte(m.Data.Credentials),
		},
	}
	// Set mondoo instance as the owner and controller
	ctrl.SetControllerReference(m, dep, r.Scheme)

	return dep
}

func (r *MondooClientReconciler) configMapForMondooDaemonSet(m *v1alpha1.MondooClient, name string, defaultInventory string) *corev1.ConfigMap {
	var inventory string
	if m.Data.KubeNodes.Inventory == "" {
		inventory = defaultInventory
	} else {
		inventory = m.Data.KubeNodes.Inventory
	}
	dep := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.Namespace,
		},
		Data: map[string]string{
			"inventory": inventory,
		},
	}
	// Set mondoo instance as the owner and controller
	ctrl.SetControllerReference(m, dep, r.Scheme)

	return dep
}
func (r *MondooClientReconciler) configMapForMondooDeployment(m *v1alpha1.MondooClient, name string, defaultInventory string) *corev1.ConfigMap {
	var inventory string
	if m.Data.Kubeapi.Inventory == "" {
		inventory = defaultInventory
	} else {
		inventory = m.Data.Kubeapi.Inventory
	}
	dep := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.Namespace,
		},
		Data: map[string]string{
			"inventory": inventory,
		},
	}
	// Set mondoo instance as the owner and controller
	ctrl.SetControllerReference(m, dep, r.Scheme)

	return dep
}
func (r *MondooClientReconciler) serviceAccountForMondoo(m *v1alpha1.MondooClient) *corev1.ServiceAccount {
	dep := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
	}
	// Set mondoo instance as the owner and controller
	ctrl.SetControllerReference(m, dep, r.Scheme)
	return dep
}

func (r *MondooClientReconciler) clusterRoleForMondoo(m *v1alpha1.MondooClient) *rbacv1.ClusterRole {
	dep := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: m.Name,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"get", "watch", "list"},
				APIGroups: []string{"*"},
				Resources: []string{"*"},
			},
		},
	}
	ctrl.SetControllerReference(m, dep, r.Scheme)
	return dep
}

func (r *MondooClientReconciler) clusterRoleBindingForMondoo(m *v1alpha1.MondooClient) *rbacv1.ClusterRoleBinding {
	dep := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: m.Name,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Name:     m.Name,
			Kind:     "ClusterRole",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      m.Name,
				Namespace: m.Namespace,
			},
		},
	}
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
