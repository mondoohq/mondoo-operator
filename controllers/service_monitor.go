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
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

type ServiceMonitor struct {
	Config          *mondoov1alpha2.MondooOperatorConfig
	TargetNamespace string
}

func (s *ServiceMonitor) serviceMonitorName() string {
	return "mondoo-operator-metrics-monitor"
}

func (s *ServiceMonitor) declareServiceMonitor(ctx context.Context, clt client.Client, scheme *runtime.Scheme) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	found := &monitoringv1.ServiceMonitor{}
	err := clt.Get(ctx, types.NamespacedName{Name: s.serviceMonitorName(), Namespace: s.TargetNamespace}, found)
	if err != nil && errors.IsNotFound(err) {

		declared := s.serviceMonitorForMondoo(s.Config)
		if err := ctrl.SetControllerReference(s.Config, declared, scheme); err != nil {
			log.Error(err, "Failed to set ControllerReference", "ServiceMonitor.Namespace", declared.Namespace, "ServiceMonitor.Name", declared.Name)
			return ctrl.Result{}, err
		}

		err := clt.Create(ctx, declared)
		if err != nil {
			log.Error(err, "Failed to create new ServiceMonitor", "ServiceMonitor.Namespace", declared.Namespace, "ServiceMonitor.Name", declared.Name)
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, err

	} else if err == nil {

		declared := s.serviceMonitorForMondoo(s.Config)
		if !reflect.DeepEqual(found.Spec, declared.Spec) {
			found.Spec = declared.Spec
			err = clt.Update(ctx, found)
			if err != nil {
				log.Error(err, "Failed to update ServiceMonitor", "ServiceMonitor.Namespace", declared.Namespace, "ServiceMonitor.Name", declared.Name)
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, err

	} else if err != nil {
		log.Error(err, "Failed to get ServiceMonitor")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (s *ServiceMonitor) serviceMonitorForMondoo(m *mondoov1alpha2.MondooOperatorConfig) *monitoringv1.ServiceMonitor {
	ls := labelsForMondoo(m.Name)
	for key, value := range s.Config.Spec.Metrics.ResourceLabels {
		ls[key] = value
	}
	dep := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.serviceMonitorName(),
			Namespace: s.TargetNamespace,
			Labels:    ls,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Endpoints: []monitoringv1.Endpoint{
				{
					Path: "/metrics",
					// The named port exposing metrics from the mondoo-operator Deployment
					Port:   "metrics",
					Scheme: "http",
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					// The key/value set for the mondoo-operator Deployment
					"app.kubernetes.io/name": "mondoo-operator",
				},
			},
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{m.Namespace},
			},
		},
	}
	return dep
}

func (s *ServiceMonitor) Reconcile(ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	found, err := k8s.VerifyAPI(monitoringv1.SchemeGroupVersion.Group, monitoringv1.SchemeGroupVersion.Version, log)
	if err != nil {
		return ctrl.Result{}, err
	}
	if s.Config.Spec.Metrics.Enable {
		// Update MondooOperatorConfig.Status to communicate why metrics could/couldn't be configured
		config := s.Config.DeepCopy()
		updatePrometheusNotInstalledCondition(config, found)

		if err := mondoo.UpdateMondooOperatorConfigStatus(ctx, clt, s.Config, config, log); err != nil {
			return ctrl.Result{}, err
		}

		if !found {
			// exit early as there is no ServiceMonitor CRD
			return ctrl.Result{}, nil
		}
		// Create/Update the ServiceMonitor
		result, err := s.declareServiceMonitor(ctx, clt, scheme)
		if err != nil || result.Requeue {
			return result, err
		}
	} else {
		if found {
			return s.down(ctx, clt)
		}
	}
	return ctrl.Result{}, nil
}

func updatePrometheusNotInstalledCondition(config *mondoov1alpha2.MondooOperatorConfig, found bool) {
	msg := "Prometheus installation detected"
	reason := "PrometheusFound"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	if !found {
		msg = "Prometheus installation not detected"
		reason = "PrometheusMissing"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = mondoo.SetMondooOperatorConfigCondition(
		config.Status.Conditions, mondoov1alpha2.PrometheusMissingCondition, status, reason, msg, updateCheck)
}

func (s *ServiceMonitor) down(ctx context.Context, clt client.Client) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	found := &monitoringv1.ServiceMonitor{}
	err := clt.Get(ctx, types.NamespacedName{Name: s.serviceMonitorName(), Namespace: s.TargetNamespace}, found)

	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get ServiceMonitor")
		return ctrl.Result{}, err
	}

	// If the ServiceMonitor was created by mondoo-operator then delete it
	if found.Labels["app"] == "mondoo" {
		if err := clt.Delete(ctx, found); err != nil {
			log.Error(err, "Failed to delete ServiceMonitor", "ServiceMonitor.Namespace", found.Namespace, "ServiceMonitor.Name", found.Name)
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{Requeue: true}, err
}
