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
	"time"

	"go.mondoo.com/mondoo-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

type Nodes struct {
	Enable  bool
	Mondoo  v1alpha1.MondooAuditConfig
	Updated bool
}

func (n *Nodes) DeclareConfigMap(ctx context.Context, clt client.Client, scheme runtime.Scheme, req ctrl.Request, inventory string) (ctrl.Result, error) {

	log := ctrllog.FromContext(ctx)

	var found corev1.ConfigMap
	req.NamespacedName.Name = n.Mondoo.Name + "-ds"
	err := clt.Get(ctx, req.NamespacedName, &found)
	if err != nil && errors.IsNotFound(err) {
		found.ObjectMeta = metav1.ObjectMeta{
			Name:      req.NamespacedName.Name,
			Namespace: req.NamespacedName.Namespace,
		}
		found.Data = map[string]string{
			"inventory": inventory,
		}
		ctrl.SetControllerReference(&n.Mondoo, &found, scheme.Scheme)
		err := clt.Create(ctx, &found)
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

		err := clt.Update(ctx, &found)
		if err != nil {
			log.Error(err, "Failed to update Configmap", "ConfigMap.Namespace", found.Namespace, "ConfigMap.Name", found.Name)
			return ctrl.Result{}, err
		}
		n.Updated = true
		return ctrl.Result{Requeue: true}, err
	}

	return ctrl.Result{}, nil
}

func (n *Nodes) DeclareDaemonSet(ctx context.Context, clt client.Client, req ctrl.Request, update bool) (ctrl.Result, error) {

	log := ctrllog.FromContext(ctx)

	var found appsv1.DaemonSet
	req.NamespacedName.Name = n.Mondoo.Name
	err := clt.Get(ctx, req.NamespacedName, &found)

	if err != nil && errors.IsNotFound(err) {
		found.ObjectMeta = metav1.ObjectMeta{
			Name:      req.NamespacedName.Name,
			Namespace: req.NamespacedName.Namespace,
		}
		err := clt.Create(ctx, &found)
		if err != nil {
			log.Error(err, "Failed to create new Configmap", "ConfigMap.Namespace", found.Namespace, "ConfigMap.Name", found.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, err

	} else if err != nil {
		log.Error(err, "Failed to get Configmap")
		return ctrl.Result{}, err
	} else if err == nil && n.Updated {
		if found.Spec.Template.ObjectMeta.Annotations == nil {
			annotation := map[string]string{
				"kubectl.kubernetes.io/restartedAt": metav1.Time{Time: time.Now()}.String(),
			}

			found.Spec.Template.ObjectMeta.Annotations = annotation
		} else if found.Spec.Template.ObjectMeta.Annotations != nil {
			found.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = metav1.Time{Time: time.Now()}.String()
		}
		err := clt.Update(ctx, &found)
		if err != nil {
			log.Error(err, "failed to restart daemonset", "Daemonset.Namespace", found.Namespace, "Dameonset.Name", found.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, err
	}

	return ctrl.Result{}, nil
}

func (n *Nodes) Down(ctx context.Context, clt client.Client, req ctrl.Request) (ctrl.Result, error) {

	log := ctrllog.FromContext(ctx)
	finalizer := "batch.tutorial.kubebuilder.io/finalizer"

	var found appsv1.DaemonSet
	req.NamespacedName.Name = n.Mondoo.Name
	err := clt.Get(ctx, req.NamespacedName, &found)

	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get DaemonSet")
		return ctrl.Result{}, err
	} else if err == nil {
		err := clt.Delete(ctx, &found)
		if err != nil {
			log.Error(err, "Failed to delete Daemonset", "Daemonset.Namespace", found.Namespace, "Daemonset.Name", found.Name)
			return ctrl.Result{}, err
		}
		if controllerutil.ContainsFinalizer(&found, finalizer) {
			// our finalizer is present, so lets handle any external dependency
			if _, err := n.deleteExternalResources(ctx, clt, req, &found); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return ctrl.Result{}, err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(&found, finalizer)
			if err := clt.Update(ctx, &found); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{Requeue: true}, err
	}
	return ctrl.Result{}, nil
}

func (n *Nodes) Up(ctx context.Context, clt client.Client, scheme runtime.Scheme, req ctrl.Request, inventory string) (ctrl.Result, error) {

	if n.Enable {
		n.DeclareConfigMap(ctx, clt, scheme, req, inventory)
		n.DeclareDaemonSet(ctx, clt, req, true)
	} else {
		n.Down(ctx, clt, req)
	}
	return ctrl.Result{}, nil
}

func (n *Nodes) deleteExternalResources(ctx context.Context, clt client.Client, req ctrl.Request, DaemonSet *appsv1.DaemonSet) (ctrl.Result, error) {
	//
	// delete any external resources associated with the cronJob
	//
	// Ensure that delete implementation is idempotent and safe to invoke
	// multiple times for same object.

	log := ctrllog.FromContext(ctx)
	var found corev1.ConfigMap
	req.NamespacedName.Name = n.Mondoo.Name + "-ds"
	err := clt.Get(ctx, req.NamespacedName, &found)
	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get ConfigMap")
		return ctrl.Result{}, err
	} else if err == nil {
		err := clt.Delete(ctx, &found)
		if err != nil {
			log.Error(err, "Failed to delete ConfigMap", "ConfigMap.Namespace", found.Namespace, "ConfigMap.Name", found.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, err
	}
	return ctrl.Result{}, nil
}
