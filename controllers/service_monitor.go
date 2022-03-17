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

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"go.mondoo.com/mondoo-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

type ServiceMonitor struct {
	Enable bool
	Mondoo v1alpha1.MondooAuditConfig
}

func (s *ServiceMonitor) declareServiceMonitor(ctx context.Context, clt client.Client, scheme *runtime.Scheme) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	found := &monitoringv1.ServiceMonitor{}
	err := clt.Get(ctx, types.NamespacedName{Name: s.Mondoo.Name + "-metrics-monitor", Namespace: s.Mondoo.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {

		declared := s.serviceMonitorForMondoo(&s.Mondoo)
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

	} else if err == nil {

		declared := s.serviceMonitorForMondoo(&s.Mondoo)
		err = clt.Update(ctx, declared)
		if err != nil {
			log.Error(err, "Failed to update ServiceMonitor", "ServiceMonitor.Namespace", declared.Namespace, "ServiceMonitor.Name", declared.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err

	} else if err != nil {
		log.Error(err, "Failed to get ServiceMonitor")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (s *ServiceMonitor) serviceMonitorForMondoo(m *v1alpha1.MondooAuditConfig) *monitoringv1.ServiceMonitor {
	ls := labelsForMondoo(m.Name)
	dep := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name + "-metrics-monitor",
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
					"control-plane": "controller-manager",
				},
			},
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{m.Namespace},
			},
		},
	}
	return dep
}
func (s *ServiceMonitor) Reconcile(ctx context.Context, clt client.Client, scheme *runtime.Scheme, req ctrl.Request, r *MondooAuditConfigReconciler) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	found, err := verifyAPI(monitoringv1.SchemeGroupVersion.Group, monitoringv1.SchemeGroupVersion.Version, ctx)

	if err != nil {
		return ctrl.Result{}, err
	}
	if s.Enable {
		// Update MondooAuditConfig.Status to communicate why metrics couldn't be configured
		mondoo := &v1alpha1.MondooAuditConfig{}

		err := clt.Get(ctx, req.NamespacedName, mondoo)
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
		if !found {
			mondoo.Status.PrometheusApiStatus = "Prometheus API Not Found"
			log.Info("Prometheus API Not found")
			r.Status().Update(ctx, mondoo)
			if err != nil {
				log.Error(err, "Failed to update Mondoo Status", "Mondoo.Namespace", mondoo.Namespace, "Mondoo.Name", mondoo.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		} else {
			// Update MondoAuditConfig.Status to say that we are not blocked on prometheus CRDs not being installed (in case we were previously in an error state we need to be sure to clear it here)
			mondoo.Status.PrometheusApiStatus = "Prometheus API Found"
			log.Info("Prometheus API found")
			r.Status().Update(ctx, mondoo)
			if err != nil {
				log.Error(err, "Failed to update Mondoo Status", "Mondoo.Namespace", mondoo.Namespace, "Mondoo.Name", mondoo.Name)
				return ctrl.Result{}, err
			}
		}
		result, err := s.declareServiceMonitor(ctx, clt, scheme)
		if err != nil || result.Requeue {
			return result, err
		}
	} else {
		if found {
			s.down(ctx, clt)
		}
	}
	return ctrl.Result{}, nil
}

func (s *ServiceMonitor) down(ctx context.Context, clt client.Client) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	found := &monitoringv1.ServiceMonitor{}
	err := clt.Get(ctx, types.NamespacedName{Name: s.Mondoo.Name + "-metrics-monitor", Namespace: s.Mondoo.Namespace}, found)

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

// Verify if Prometheus API exists
func verifyAPI(group string, version string, ctx context.Context) (bool, error) {
	log := ctrllog.FromContext(ctx)
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "unable to get k8s config")
		return false, err
	}

	k8s, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "unable to create k8s client")
		return false, err
	}

	gv := schema.GroupVersion{
		Group:   group,
		Version: version,
	}

	if err = discovery.ServerSupportsVersion(k8s, gv); err != nil {
		// error, API not available
		return false, nil
	}

	log.Info(fmt.Sprintf("%s/%s API verified", group, version))
	return true, nil
}
