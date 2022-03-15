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

	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	"go.mondoo.com/mondoo-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

type ServiceMonitor struct {
	Enable bool
	Mondoo v1alpha1.MondooAuditConfig
}

func (s *ServiceMonitor) declareServiceMonitor(ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	found := &monitoringv1.ServiceMonitor{}
	err := clt.Get(ctx, types.NamespacedName{Name: s.Mondoo.Name, Namespace: s.Mondoo.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {

		declared := s.serviceMonitorForMondoo(&s.Mondoo, s.Mondoo.Name)
		if err := ctrl.SetControllerReference(&s.Mondoo, declared, scheme); err != nil {
			log.Error(err, "Failed to set ControllerReference", "ServiceMonitor.Namespace", declared.Namespace, "ServiceMonitor.Name", declared.Name)
			return ctrl.Result{}, err
		}

		err := clt.Create(ctx, declared)
		if err != nil {
			log.Error(err, "Failed to create new ServiceMonitor", "ServiceMonitor.Namespace", declared.Namespace, "ServiceMonitor.Name", declared.Name)
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, err

	} else if err != nil {
		log.Error(err, "Failed to get ServiceMonitor")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (s *ServiceMonitor) serviceMonitorForMondoo(m *v1alpha1.MondooAuditConfig, cmName string) *monitoringv1.ServiceMonitor {
	ls := labelsForMondoo(m.Name)
	dep := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name + "metrics-monitor",
			Namespace: m.Namespace,
			Labels:    ls,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Endpoints: []monitoringv1.Endpoint{
				{
					Path:   "/metrics",
					Port:   "https",
					Scheme: "https",
					TLSConfig: &monitoringv1.TLSConfig{
						SafeTLSConfig: monitoringv1.SafeTLSConfig{
							InsecureSkipVerify: true,
						},
					},
					BearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"control_plane": "controller_manager",
				},
			},
		},
	}
	return dep
}
func (s *ServiceMonitor) Reconcile(ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request) (ctrl.Result, error) {
	if s.Enable {
		result, err := s.declareServiceMonitor(ctx, clt, scheme, req)
		if err != nil || result.Requeue {
			return result, err
		}
	} else {
		s.down(ctx, clt, req)
	}
	return ctrl.Result{}, nil
}

func (s *ServiceMonitor) down(ctx context.Context, clt client.Client, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	found := &monitoringv1.ServiceMonitor{}
	err := clt.Get(ctx, types.NamespacedName{Name: s.Mondoo.Name, Namespace: s.Mondoo.Namespace}, found)

	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get ServiceMonitor")
		return ctrl.Result{}, err
	}

	err = clt.Delete(ctx, found)
	if err != nil {
		log.Error(err, "Failed to delete ServiceMonitor", "ServiceMonitor.Namespace", found.Namespace, "ServiceMonitor.Name", found.Name)
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, err
}
