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
	"time"

	"go.mondoo.com/mondoo-operator/api/v1alpha1"
	mondoov1alpha1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

type Workloads struct {
	Enable               bool
	Mondoo               *v1alpha1.MondooAuditConfig
	Updated              bool
	Image                string
	MondooOperatorConfig *v1alpha1.MondooOperatorConfig
}

func (n *Workloads) declareConfigMap(ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request, inventory string) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	found := &corev1.ConfigMap{}
	err := clt.Get(ctx, types.NamespacedName{Name: n.Mondoo.Name + "-deploy", Namespace: n.Mondoo.Namespace}, found)

	if n.Mondoo.Spec.Workloads.Inventory != "" {
		inventory = n.Mondoo.Spec.Workloads.Inventory
	}
	if err != nil && errors.IsNotFound(err) {
		found.ObjectMeta = metav1.ObjectMeta{
			Name:      req.NamespacedName.Name + "-deploy",
			Namespace: req.NamespacedName.Namespace,
		}
		found.Data = map[string]string{
			"inventory": inventory,
		}
		if err := ctrl.SetControllerReference(n.Mondoo, found, scheme); err != nil {
			log.Error(err, "Failed to set ControllerReference", "ConfigMap.Namespace", found.Namespace, "ConfigMap.Name", found.Name)
			return ctrl.Result{}, err
		}

		err := clt.Create(ctx, found)
		if err != nil {
			log.Error(err, "Failed to create new Configmap", "ConfigMap.Namespace", found.Namespace, "ConfigMap.Name", found.Name)
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, err

	} else if err != nil {
		log.Error(err, "Failed to get Configmap")
		return ctrl.Result{}, err
	} else if err == nil && found.Data["inventory"] != inventory {
		found.Data = map[string]string{
			"inventory": inventory,
		}

		err := clt.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update Configmap", "ConfigMap.Namespace", found.Namespace, "ConfigMap.Name", found.Name)
			return ctrl.Result{}, err
		}
		n.Updated = true
		return ctrl.Result{Requeue: true}, err
	}

	return ctrl.Result{}, nil
}

func (n *Workloads) declareDeployment(ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request, update bool) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	found := &appsv1.Deployment{}
	err := clt.Get(ctx, types.NamespacedName{Name: n.Mondoo.Name, Namespace: n.Mondoo.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {

		declared := n.deploymentForMondoo(n.Mondoo, n.Mondoo.Name+"-deploy")
		if err := ctrl.SetControllerReference(n.Mondoo, declared, scheme); err != nil {
			log.Error(err, "Failed to set ControllerReference", "Deployment.Namespace", declared.Namespace, "Deployment.Name", declared.Name)
			return ctrl.Result{}, err
		}

		err := clt.Create(ctx, declared)
		if err != nil {
			log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", declared.Namespace, "Deployment.Name", declared.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, err

	} else if err == nil && found.Spec.Template.Spec.Containers[0].Image != n.Image {
		found.Spec.Template.Spec.Containers[0].Image = n.Image
		err := clt.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update Deployment", "Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, err
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	// check that the resource limites are identical
	expectedResourceRequirements := getResourcesRequirements(n.Mondoo.Spec.Workloads.Resources)
	if !equalResouceRequirements(found.Spec.Template.Spec.Containers[0].Resources, expectedResourceRequirements) {
		log.Info("update resource requirements for workload client")
		found.Spec.Template.Spec.Containers[0].Resources = expectedResourceRequirements
		err := clt.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update Deployment", "Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, err
	}

	if n.Updated {
		if found.Spec.Template.ObjectMeta.Annotations == nil {
			annotation := map[string]string{
				"kubectl.kubernetes.io/restartedAt": metav1.Time{Time: time.Now()}.String(),
			}

			found.Spec.Template.ObjectMeta.Annotations = annotation
		} else if found.Spec.Template.ObjectMeta.Annotations != nil {
			found.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = metav1.Time{Time: time.Now()}.String()
		}
		err := clt.Update(ctx, found)
		if err != nil {
			log.Error(err, "failed to restart Deployment", "Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, err
	}

	config := n.Mondoo.DeepCopy()
	updateWorkloadsConditions(config, found)
	if err := UpdateMondooAuditStatus(ctx, clt, &n.Mondoo, config, log); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// deploymentForMondoo returns a Deployment object
func (n *Workloads) deploymentForMondoo(m *v1alpha1.MondooAuditConfig, cmName string) *appsv1.Deployment {
	ls := labelsForMondoo(m.Name)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
			Labels:    ls,
		},
		Spec: appsv1.DeploymentSpec{
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
						Image:     n.Image,
						Name:      "mondoo-client",
						Command:   []string{"mondoo", "serve", "--config", "/etc/opt/mondoo/mondoo.yml"},
						Resources: getResourcesRequirements(m.Spec.Workloads.Resources),
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"mondoo", "status"},
								},
							},
							InitialDelaySeconds: 1,
							PeriodSeconds:       1800,
							TimeoutSeconds:      10,
							SuccessThreshold:    1,
							FailureThreshold:    1,
						},
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
					ServiceAccountName: m.Spec.Workloads.ServiceAccount,
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
													Name: m.Spec.MondooSecretRef,
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
	return dep
}

func (n *Workloads) Reconcile(ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request, inventory string) (ctrl.Result, error) {

	log := ctrllog.FromContext(ctx)

	namespace, err := getNamespace()
	if err != nil {
		log.Error(err, "failed to determine which namespace mondoo-operator is running in")
		return ctrl.Result{}, err
	}

	if n.Mondoo.Spec.Workloads.ServiceAccount == "" && n.Mondoo.Namespace == namespace {
		n.Mondoo.Spec.Workloads.ServiceAccount = defaultServiceAccount
	}
	if n.Mondoo.Spec.Workloads.ServiceAccount == "" {
		err := fmt.Errorf("MondooAuditConfig.spec.workloads.serviceAccount cannot be empty when running in a different namespace than mondoo-operator")
		log.Error(err, "ServiceAccount cannot be empty")
		return ctrl.Result{}, err
	}

	if n.Enable {
		skipResolveImage := n.MondooOperatorConfig.Spec.SkipContainerResolution
		mondooImage, err := resolveMondooImage(log, n.Mondoo.Spec.Workloads.Image.Name, n.Mondoo.Spec.Workloads.Image.Tag, skipResolveImage)
		if err != nil {
			return ctrl.Result{}, err
		}
		n.Image = mondooImage

		result, err := n.declareConfigMap(ctx, clt, scheme, req, inventory)
		if err != nil || result.Requeue {
			return result, err
		}
		result, err = n.declareDeployment(ctx, clt, scheme, req, true)
		if err != nil || result.Requeue {
			return result, err
		}
	} else {
		n.down(ctx, clt, req)
	}
	return ctrl.Result{}, nil
}

func (n *Workloads) down(ctx context.Context, clt client.Client, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	found := &appsv1.Deployment{}
	err := clt.Get(ctx, types.NamespacedName{Name: n.Mondoo.Name, Namespace: n.Mondoo.Namespace}, found)

	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	err = clt.Delete(ctx, found)
	if err != nil {
		log.Error(err, "Failed to delete Deployment", "Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)
		return ctrl.Result{}, err
	}
	if _, err := n.deleteExternalResources(ctx, clt, req, found); err != nil {
		// if fail to delete the external dependency here, return with error
		// so that it can be retried
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, err
}

// deleteExternalResources deletes any external resources associated with the Deployment
//
// Ensure that delete implementation is idempotent and safe to invoke
// multiple times for same object.
func (n *Workloads) deleteExternalResources(ctx context.Context, clt client.Client, req ctrl.Request, Deployment *appsv1.Deployment) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	found := &corev1.ConfigMap{}
	err := clt.Get(ctx, types.NamespacedName{Name: n.Mondoo.Name + "-deploy", Namespace: n.Mondoo.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get ConfigMap")
		return ctrl.Result{}, err
	}

	err = clt.Delete(ctx, found)
	if err != nil {
		log.Error(err, "Failed to delete ConfigMap", "ConfigMap.Namespace", found.Namespace, "ConfigMap.Name", found.Name)
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, err
}

func updateWorkloadsConditions(config *mondoov1alpha1.MondooAuditConfig, found *appsv1.Deployment) {
	msg := "API Scanning is unavailable"
	reason := "APIScanningUnvailable"
	status := corev1.ConditionTrue
	updateCheck := UpdateConditionIfReasonOrMessageChange
	if found.Status.Replicas == found.Status.ReadyReplicas {
		msg = "API Scanning is available"
		reason = "APIScanningAvailable"
		status = corev1.ConditionFalse
	}

	config.Status.Conditions = SetMondooAuditCondition(config.Status.Conditions, mondoov1alpha1.APIScanningDegraded, status, reason, msg, updateCheck)

}
