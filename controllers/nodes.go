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

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	daemonSetConfigMapNameTemplate = `%s-ds`
	NodeDaemonSetNameTemplate      = `%s-node`
)

type Nodes struct {
	Enable                 bool
	Mondoo                 *v1alpha2.MondooAuditConfig
	Updated                bool
	ContainerImageResolver mondoo.ContainerImageResolver
	MondooOperatorConfig   *v1alpha2.MondooOperatorConfig
}

func (n *Nodes) declareConfigMap(ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request, inventory string) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	configMapName := fmt.Sprintf(daemonSetConfigMapNameTemplate, n.Mondoo.Name)

	found := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: n.Mondoo.Namespace,
			Name:      configMapName,
		},
	}
	err := clt.Get(ctx, client.ObjectKeyFromObject(found), found)

	if err != nil && errors.IsNotFound(err) {
		found.ObjectMeta = metav1.ObjectMeta{
			Name:      configMapName,
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

func (n *Nodes) declareDaemonSet(ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request, update bool) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	mondooClientImage, err := n.ContainerImageResolver.MondooClientImage(
		n.Mondoo.Spec.Scanner.Image.Name, n.Mondoo.Spec.Scanner.Image.Tag, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		return ctrl.Result{}, err
	}

	found := &appsv1.DaemonSet{}
	err = clt.Get(ctx, types.NamespacedName{Name: fmt.Sprintf(NodeDaemonSetNameTemplate, n.Mondoo.Name), Namespace: n.Mondoo.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {

		declared := n.daemonsetForMondoo(mondooClientImage)
		if err := ctrl.SetControllerReference(n.Mondoo, declared, scheme); err != nil {
			log.Error(err, "Failed to set ControllerReference", "Daemonset.Namespace", declared.Namespace, "Daemonset.Name", declared.Name)
			return ctrl.Result{}, err
		}

		err := clt.Create(ctx, declared)
		if err != nil {
			log.Error(err, "Failed to create new Daemonset", "Daemonset.Namespace", declared.Namespace, "Daemonset.Name", declared.Name)
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, err

	} else if err == nil && found.Spec.Template.Spec.Containers[0].Image != mondooClientImage {
		found.Spec.Template.Spec.Containers[0].Image = mondooClientImage
		err := clt.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update Daemonset", "Daemonset.Namespace", found.Namespace, "Daemonset.Name", found.Name)
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, err
	} else if err != nil {
		log.Error(err, "Failed to get Daemonset")
		return ctrl.Result{}, err
	}

	// check that the resource limites are identical
	expectedResourceRequirements := k8s.ResourcesRequirementsWithDefaults(n.Mondoo.Spec.Scanner.Resources)
	if !k8s.AreResouceRequirementsEqual(found.Spec.Template.Spec.Containers[0].Resources, expectedResourceRequirements) {
		log.Info("update resource requirements for nodes client")
		found.Spec.Template.Spec.Containers[0].Resources = expectedResourceRequirements
		err := clt.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update Daemonset", "Daemonset.Namespace", found.Namespace, "Daemonset.Name", found.Name)
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
			log.Error(err, "failed to restart daemonset", "Daemonset.Namespace", found.Namespace, "Dameonset.Name", found.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, err
	}
	updateNodeConditions(n.Mondoo, found.Status.NumberReady != found.Status.DesiredNumberScheduled)

	err = n.cleanupOldDaemonSet(ctx, clt)

	return ctrl.Result{}, err
}

func (n *Nodes) daemonsetForMondoo(image string) *appsv1.DaemonSet {
	ls := labelsForMondoo(n.Mondoo.Name)
	ls["audit"] = "node"

	dep := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(NodeDaemonSetNameTemplate, n.Mondoo.Name),
			Namespace: n.Mondoo.Namespace,
			Labels:    ls,
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
					Tolerations: []corev1.Toleration{
						{
							Key:    "node-role.kubernetes.io/master",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							// Rancher etcd node
							// https://rancher.com/docs/rke/latest/en/config-options/nodes/#etcd
							Key:    "node-role.kubernetes.io/etcd",
							Effect: corev1.TaintEffectNoExecute,
							Value:  "true",
						},
						{
							// Rancher controlplane node
							// https://rancher.com/docs/rke/latest/en/config-options/nodes/#controlplane
							Key:    "node-role.kubernetes.io/controlplane",
							Effect: corev1.TaintEffectNoSchedule,
							Value:  "true",
						},
					},
					// The node scanning does not use the Kubernetes API at all, therefore the service account token
					// should not be mounted at all.
					AutomountServiceAccountToken: pointer.Bool(false),
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
													Name: fmt.Sprintf(daemonSetConfigMapNameTemplate, n.Mondoo.Name),
												},
												Items: []corev1.KeyToPath{{
													Key:  "inventory",
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
func (n *Nodes) Reconcile(ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request, inventory string) (ctrl.Result, error) {
	if !n.Enable {
		return n.down(ctx, clt, req)
	}

	result, err := n.declareConfigMap(ctx, clt, scheme, req, inventory)
	if err != nil || result.Requeue {
		return result, err
	}
	result, err = n.declareDaemonSet(ctx, clt, scheme, req, true)
	if err != nil || result.Requeue {
		return result, err
	}
	return ctrl.Result{}, nil
}

func (n *Nodes) down(ctx context.Context, clt client.Client, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	found := &appsv1.DaemonSet{}
	err := clt.Get(ctx, types.NamespacedName{Name: fmt.Sprintf(NodeDaemonSetNameTemplate, n.Mondoo.Name), Namespace: n.Mondoo.Namespace}, found)

	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Daemonset")
		return ctrl.Result{}, err
	}

	err = clt.Delete(ctx, found)
	if err != nil {
		log.Error(err, "Failed to delete Daemonset", "Daemonset.Namespace", found.Namespace, "Daemonset.Name", found.Name)
		return ctrl.Result{}, err
	}
	if _, err := n.deleteExternalResources(ctx, clt, req, found); err != nil {
		// if fail to delete the external dependency here, return with error
		// so that it can be retried
		return ctrl.Result{}, err
	}

	if err := n.cleanupOldDaemonSet(ctx, clt); err != nil {
		return ctrl.Result{}, err
	}

	// Update any remnant conditions
	updateNodeConditions(n.Mondoo, false)

	return ctrl.Result{Requeue: true}, err
}

// deleteExternalResources deletes any external resources associated with the daemonset
func (n *Nodes) deleteExternalResources(ctx context.Context, clt client.Client, req ctrl.Request, DaemonSet *appsv1.DaemonSet) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	found := &corev1.ConfigMap{}
	err := clt.Get(ctx, types.NamespacedName{Name: n.Mondoo.Name + "-ds", Namespace: n.Mondoo.Namespace}, found)

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

func updateNodeConditions(config *v1alpha2.MondooAuditConfig, degradedStatus bool) {
	msg := "Node Scanning is Available"
	reason := "NodeScanningAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	if degradedStatus {
		msg = "Node Scanning is Unavailable"
		reason = "NodeScanningUnavailable"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.NodeScanningDegraded, status, reason, msg, updateCheck)

}

// TODO: this can be removed once we believe enough time has passed where the old-style named
// DaemonSet has been replaced and removed to keep us from orphaning the old-style DaemonSet.
func (n *Nodes) cleanupOldDaemonSet(ctx context.Context, kubeClient client.Client) error {
	log := ctrllog.FromContext(ctx)

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: n.Mondoo.Namespace,
			Name:      n.Mondoo.Name,
		},
	}

	err := k8s.DeleteIfExists(ctx, kubeClient, ds)
	if err != nil {
		log.Error(err, "failed while cleaning up old DaemonSet for nodes")
	}
	return err
}
