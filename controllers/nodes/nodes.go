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

package nodes

import (
	"context"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	daemonSetConfigMapNameTemplate = `%s-ds`
	OldNodeDaemonSetNameTemplate   = `%s-node`
)

var (
	//go:embed inventory-ds.yaml
	dsInventoryyaml []byte
)

var logger = ctrl.Log.WithName("node-scanning")

type Nodes struct {
	KubeClient             client.Client
	Mondoo                 *v1alpha2.MondooAuditConfig
	ContainerImageResolver mondoo.ContainerImageResolver
	MondooOperatorConfig   *v1alpha2.MondooOperatorConfig
}

func (n *Nodes) Reconcile(ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request) (ctrl.Result, error) {
	if !n.Mondoo.Spec.Nodes.Enable {
		return ctrl.Result{}, n.down(ctx, req)
	}

	updated, err := n.syncConfigMap(ctx, req)
	if err != nil {
		return ctrl.Result{}, err
	}

	if updated {
		logger.Info("Inventory ConfigMap was just updated. Running node scanning job now...")
	}

	err = n.syncCronJob(ctx, req)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// syncConfigMap syncs the inventory ConfigMap. Returns a boolean indicating whether the ConfigMap has been updated. It
// can only be "true", if the ConfigMap existed before this reconcile cycle and the inventory was different from the
// desired state.
func (n *Nodes) syncConfigMap(ctx context.Context, req ctrl.Request) (bool, error) {
	existing := &corev1.ConfigMap{}
	desired := ConfigMap(*n.Mondoo)
	if err := ctrl.SetControllerReference(n.Mondoo, desired, n.KubeClient.Scheme()); err != nil {
		logger.Error(err, "Failed to set ControllerReference", "namespace", desired.Namespace, "name", desired.Name)
		return false, err
	}

	created, err := k8s.CreateIfNotExist(ctx, n.KubeClient, existing, desired)
	if err != nil {
		logger.Error(err, "Failed to create inventory ConfigMap", "namespace", desired.Namespace, "name", desired.Name)
		return false, err
	}

	if created {
		logger.Info("Created inventory ConfigMap", "namespace", desired.Namespace, "name", desired.Name)
		return false, nil
	}

	updated := false
	if existing.Data["inventory"] != desired.Data["inventory"] {
		existing.Data["inventory"] = desired.Data["inventory"]

		if err := n.KubeClient.Update(ctx, existing); err != nil {
			logger.Error(err, "Failed to update inventory ConfigMap", "namespace", existing.Namespace, "name", existing.Name)
			return false, err
		}
		updated = true
	}
	return updated, nil
}

func (n *Nodes) syncCronJob(ctx context.Context, req ctrl.Request) error {
	mondooClientImage, err := n.ContainerImageResolver.MondooClientImage(
		n.Mondoo.Spec.Scanner.Image.Name, n.Mondoo.Spec.Scanner.Image.Tag, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		logger.Error(err, "Failed to resolve mondoo-client container image")
		return err
	}

	nodes := &corev1.NodeList{}
	if err := n.KubeClient.List(ctx, nodes); err != nil {
		logger.Error(err, "Failed to list cluster nodes")
		return err
	}

	for _, node := range nodes.Items {
		existing := &batchv1.CronJob{}
		desired := CronJob(mondooClientImage, node, *n.Mondoo)
		if err := ctrl.SetControllerReference(n.Mondoo, desired, n.KubeClient.Scheme()); err != nil {
			logger.Error(err, "Failed to set ControllerReference", "namespace", desired.Namespace, "name", desired.Name)
			return err
		}

		created, err := k8s.CreateIfNotExist(ctx, n.KubeClient, existing, desired)
		if err != nil {
			logger.Error(err, "Failed to create inventory ConfigMap", "namespace", desired.Namespace, "name", desired.Name)
			return err
		}

		if created {
			logger.Info("Created CronJob", "namespace", desired.Namespace, "name", desired.Name)
			continue
		}

		// TODO: implement deep equals for cronjobs
		if existing.Name != desired.Name {
			if err := n.KubeClient.Update(ctx, existing); err != nil {
				logger.Error(err, "Failed to update CronJob", "namespace", existing.Namespace, "name", existing.Name)
				return err
			}
		}
	}

	// TODO: for CronJob we might consider triggering the CronJob now after the ConfigMap has been changed. It will make sense from the
	// user perspective to want to run the jobs after you have updated the config.

	// TODO: figure out how to check the node scanning status
	//updateNodeConditions(n.Mondoo, found.Status.NumberReady != found.Status.DesiredNumberScheduled)
	return n.cleanupOldDaemonSet(ctx)
}

func (n *Nodes) down(ctx context.Context, req ctrl.Request) error {
	nodes := &corev1.NodeList{}
	if err := n.KubeClient.List(ctx, nodes); err != nil {
		logger.Error(err, "Failed to list cluster nodes")
		return err
	}

	for _, node := range nodes.Items {
		cronJob := &batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{Name: CronJobName(n.Mondoo.Name, node.Name), Namespace: n.Mondoo.Namespace},
		}
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, cronJob); err != nil {
			logger.Error(err, "Failed to clean up node scanning CronJob", "namespace", cronJob.Namespace, "name", cronJob.Name)
			return err
		}
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: ConfigMapName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, configMap); err != nil {
		logger.Error(err, "Failed to clean up inventory ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
		return err
	}

	if err := n.cleanupOldDaemonSet(ctx); err != nil {
		return err
	}

	// Update any remnant conditions
	updateNodeConditions(n.Mondoo, false)

	return nil
}

// TODO: this should now delete the current daemon set and replace it with the cronjob
// TODO: this can be removed once we believe enough time has passed where the old-style named
// DaemonSet has been replaced and removed to keep us from orphaning the old-style DaemonSet.
func (n *Nodes) cleanupOldDaemonSet(ctx context.Context) error {
	log := ctrllog.FromContext(ctx)

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: n.Mondoo.Namespace,
			Name:      n.Mondoo.Name,
		},
	}

	err := k8s.DeleteIfExists(ctx, n.KubeClient, ds)
	if err != nil {
		log.Error(err, "failed while cleaning up old DaemonSet for nodes")
	}
	return err
}
