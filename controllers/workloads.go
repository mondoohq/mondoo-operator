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
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/inventory"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

const (
	workloadDeploymentConfigMapNameTemplate = `%s-deploy`
	WorkloadDeploymentNameTemplate          = `%s-workload`
	configMapInventoryKey                   = "inventory"
)

type Workloads struct {
	Enable                 bool
	Mondoo                 *v1alpha2.MondooAuditConfig
	configMapHash          string
	ContainerImageResolver mondoo.ContainerImageResolver
	MondooOperatorConfig   *v1alpha2.MondooOperatorConfig
}

func (n *Workloads) declareConfigMap(ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	found := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(workloadDeploymentConfigMapNameTemplate, n.Mondoo.Name),
			Namespace: n.Mondoo.Namespace,
		},
	}

	// calculate SHA for ConfigMap contents
	defer func() {
		cm := &corev1.ConfigMap{}
		if err := clt.Get(ctx, client.ObjectKeyFromObject(found), cm); err != nil {
			log.Error(err, "failed to get ConfigMap to calculate SHA")
			return
		}
		inventoryData := cm.Data[configMapInventoryKey]

		md5sum := md5.Sum([]byte(inventoryData))
		n.configMapHash = hex.EncodeToString(md5sum[:])
	}()

	// generate the Mondoo client inventory
	inventoryData, err := defaultMondooInventory(ctx, clt, n.Mondoo)
	if err != nil {
		log.Error(err, "failed to build Mondoo Inventory for workload scanning")
		return ctrl.Result{}, err
	}

	inventoryBytes, err := yaml.Marshal(inventoryData)
	if err != nil {
		log.Error(err, "failed to marshal inventory to yaml")
		return ctrl.Result{}, err
	}
	inventory := string(inventoryBytes)

	err = clt.Get(ctx, client.ObjectKeyFromObject(found), found)
	if err != nil && errors.IsNotFound(err) {
		found.Data = map[string]string{
			configMapInventoryKey: inventory,
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

		return ctrl.Result{}, err

	} else if err != nil {
		log.Error(err, "Failed to get Configmap")
		return ctrl.Result{}, err
	} else if err == nil && found.Data[configMapInventoryKey] != inventory {
		found.Data = map[string]string{
			configMapInventoryKey: inventory,
		}

		err := clt.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update Configmap", "ConfigMap.Namespace", found.Namespace, "ConfigMap.Name", found.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (n *Workloads) declareDeployment(ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request, update bool) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	found := &appsv1.Deployment{}
	mondooClientImage, err := n.ContainerImageResolver.MondooClientImage(
		n.Mondoo.Spec.Scanner.Image.Name, n.Mondoo.Spec.Scanner.Image.Tag, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		return ctrl.Result{}, err
	}
	desiredDeployment := n.deploymentForMondoo(mondooClientImage)
	err = clt.Get(ctx, client.ObjectKeyFromObject(desiredDeployment), found)
	if err != nil && errors.IsNotFound(err) {

		if err := ctrl.SetControllerReference(n.Mondoo, desiredDeployment, scheme); err != nil {
			log.Error(err, "Failed to set ControllerReference", "Deployment.Namespace", desiredDeployment.Namespace, "Deployment.Name", desiredDeployment.Name)
			return ctrl.Result{}, err
		}

		err := clt.Create(ctx, desiredDeployment)
		if err != nil {
			log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", desiredDeployment.Namespace, "Deployment.Name", desiredDeployment.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err

	} else if err == nil && n.deploymentNeedsUpdate(desiredDeployment, found) {
		found.Spec = desiredDeployment.Spec
		err := clt.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update Deployment", "Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	updateWorkloadsConditions(n.Mondoo, found.Status.Replicas != found.Status.ReadyReplicas)

	err = n.cleanupWorkloadDeployment(ctx, clt)

	return ctrl.Result{}, err
}

func (n *Workloads) deploymentNeedsUpdate(desired, existing *appsv1.Deployment) bool {
	if existing.Spec.Template.Spec.Containers[0].Image != desired.Spec.Template.Spec.Containers[0].Image {
		return true
	}

	if existing.Spec.Template.Spec.ServiceAccountName != desired.Spec.Template.Spec.ServiceAccountName {
		return true
	}

	if !k8s.AreResouceRequirementsEqual(existing.Spec.Template.Spec.Containers[0].Resources, desired.Spec.Template.Spec.Containers[0].Resources) {
		return true
	}

	if !reflect.DeepEqual(desired.Spec.Template.ObjectMeta.Annotations, existing.Spec.Template.ObjectMeta.Annotations) {
		return true
	}

	return false
}

// deploymentForMondoo returns a Deployment object
func (n *Workloads) deploymentForMondoo(image string) *appsv1.Deployment {
	ls := labelsForMondoo(n.Mondoo.Name)
	ls["audit"] = "k8s"

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(WorkloadDeploymentNameTemplate, n.Mondoo.Name),
			Namespace: n.Mondoo.Namespace,
			Labels:    ls,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"mondoo-operator/configmap-hash": n.configMapHash,
					},
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Tolerations: []corev1.Toleration{{
						Key:    "node-role.kubernetes.io/master",
						Effect: corev1.TaintEffect("NoSchedule"),
					}},
					Containers: []corev1.Container{{
						Image:     image,
						Name:      "mondoo-client",
						Command:   []string{"mondoo", "serve", "--config", "/etc/opt/mondoo/mondoo.yml"},
						Resources: k8s.ResourcesRequirementsWithDefaults(n.Mondoo.Spec.Scanner.Resources),
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"mondoo", "status", "--config", "/etc/opt/mondoo/mondoo.yml"},
								},
							},
							InitialDelaySeconds: 10,
							PeriodSeconds:       300,
							TimeoutSeconds:      5,
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
					ServiceAccountName: n.Mondoo.Spec.Scanner.ServiceAccountName,
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
													Name: fmt.Sprintf(workloadDeploymentConfigMapNameTemplate, n.Mondoo.Name),
												},
												Items: []corev1.KeyToPath{{
													Key:  configMapInventoryKey,
													Path: "mondoo/inventory.yml",
												}},
											},
										},
										{
											Secret: &corev1.SecretProjection{
												LocalObjectReference: n.Mondoo.Spec.MondooCredsSecretRef,
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

func (n *Workloads) Reconcile(ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request) (ctrl.Result, error) {
	if !n.Enable {
		return n.down(ctx, clt, req)
	}

	result, err := n.declareConfigMap(ctx, clt, scheme, req)
	if err != nil || result.Requeue {
		return result, err
	}
	result, err = n.declareDeployment(ctx, clt, scheme, req, true)
	if err != nil || result.Requeue {
		return result, err
	}
	return ctrl.Result{}, nil
}

func (n *Workloads) down(ctx context.Context, clt client.Client, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	found := &appsv1.Deployment{}
	err := clt.Get(ctx, types.NamespacedName{Name: fmt.Sprintf(WorkloadDeploymentNameTemplate, n.Mondoo.Name), Namespace: n.Mondoo.Namespace}, found)

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

	if err := n.cleanupWorkloadDeployment(ctx, clt); err != nil {
		return ctrl.Result{}, err
	}

	// Clear any remant status
	updateWorkloadsConditions(n.Mondoo, false)

	return ctrl.Result{}, err
}

// deleteExternalResources deletes any external resources associated with the Deployment
//
// Ensure that delete implementation is idempotent and safe to invoke
// multiple times for same object.
func (n *Workloads) deleteExternalResources(ctx context.Context, clt client.Client, req ctrl.Request, Deployment *appsv1.Deployment) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	found := &corev1.ConfigMap{}
	err := clt.Get(ctx, types.NamespacedName{Name: fmt.Sprintf(workloadDeploymentConfigMapNameTemplate, n.Mondoo.Name), Namespace: n.Mondoo.Namespace}, found)
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
	return ctrl.Result{}, err
}

func updateWorkloadsConditions(config *v1alpha2.MondooAuditConfig, degradedStatus bool) {
	msg := "API Scanning is Available"
	reason := "APIScanningAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	if degradedStatus {
		msg = "API Scanning is Unavailable"
		reason = "APIScanningUnavailable"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.K8sResourcesScanningDegraded, status, reason, msg, updateCheck)

}

// TODO: this can be removed once we believe enough time has passed where the old-style named
// Deployment for workloads has been replaced and removed to keep us from orphaning the old-style Deployment.
func (n *Workloads) cleanupWorkloadDeployment(ctx context.Context, kubeClient client.Client) error {
	log := ctrllog.FromContext(ctx)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: n.Mondoo.Namespace,
			Name:      n.Mondoo.Name,
		},
	}
	err := k8s.DeleteIfExists(ctx, kubeClient, dep)
	if err != nil {
		log.Error(err, "failed while cleaning up old Deployment for workloads")
	}
	return err
}

func defaultMondooInventory(ctx context.Context, kubeClient client.Client, mac *v1alpha2.MondooAuditConfig) (*inventory.MondooInventory, error) {
	inv := &inventory.MondooInventory{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Inventory",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "mondoo-k8s-inventory",
			Labels: map[string]string{
				"environment": "production",
			},
		},
		Spec: inventory.MondooInventorySpec{
			Assets: []inventory.Asset{
				{
					Id: "api",
					Connections: []*inventory.TransportConfig{
						{
							Backend: "k8s",
						},
					},
				},
			},
		},
	}

	if mac.Spec.ConsoleIntegration.Enable {
		// Put the Integration MRN as a label
		sec := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mac.Spec.MondooCredsSecretRef.Name,
				Namespace: mac.Namespace,
			},
		}

		if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(sec), sec); err != nil {
			return nil, fmt.Errorf("failed to get Mondoo creds Secret: %s", err)
		}

		integrationMRN, ok := sec.Data[constants.MondooCredsSecretIntegrationMRNKey]
		if !ok {
			return nil, fmt.Errorf("console integration enabled and Mondoo creds Secret is missing integration MRN data expected in key '%s'", constants.MondooCredsSecretIntegrationMRNKey)
		}

		for i := range inv.Spec.Assets {
			if inv.Spec.Assets[i].Labels == nil {
				inv.Spec.Assets[i].Labels = map[string]string{}
			}
			inv.Spec.Assets[i].Labels[constants.MondooAssetsIntegrationLabel] = string(integrationMRN)
		}
	}

	return inv, nil
}
