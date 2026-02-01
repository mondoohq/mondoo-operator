// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_monitor

import (
	"context"
	"fmt"
	"strings"

	"go.mondoo.com/mondoo-operator/controllers/resource_monitor/debouncer"
	"go.mondoo.com/mondoo-operator/controllers/resource_monitor/scan_api_store"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cnquery_k8s "go.mondoo.com/cnquery/v12/providers/k8s/resources"
)

var logger = log.Log.WithName("resource-monitor")

type ResourceMonitorController struct {
	client.Client
	createRes    func() client.Object
	debouncer    debouncer.Debouncer
	resourceType string
	scanApiStore scan_api_store.ScanApiStore
}

func NewResourceMonitorController(
	kubeClient client.Client,
	createRes func() client.Object,
	scanApiStore scan_api_store.ScanApiStore,
) (*ResourceMonitorController, error) {
	gvk, err := apiutil.GVKForObject(createRes(), kubeClient.Scheme())
	if err != nil {
		logger.Error(err, "Failed to get GVK for resource") // This should never happen in practice
		panic(err)
	}

	return &ResourceMonitorController{
		Client:       kubeClient,
		createRes:    createRes,
		debouncer:    debouncer.NewDebouncer(scanApiStore),
		resourceType: strings.ToLower(gvk.Kind),
		scanApiStore: scanApiStore,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ResourceMonitorController) SetupWithManager(mgr ctrl.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(r.createRes()).
		WithEventFilter(k8s.CreateUpdateEventsPredicate{}).
		Complete(r); err != nil {
		return err
	}

	// This makes sure the debouncer is only started when the operator manager has started.
	// It also couples the lifetime of the debouncer to the lifetime of the manager.
	if err := mgr.Add(r); err != nil {
		return err
	}
	return nil
}

func (r *ResourceMonitorController) Start(ctx context.Context) error {
	clusterUid, err := k8s.GetClusterUID(ctx, r, logger)
	if err != nil {
		return err
	}

	var managedBy string
	if clusterUid == "" {
		logger.Info("no clusterUid provided, will not set ManagedBy field on scanned/discovered assets")
	} else {
		managedBy = "mondoo-operator-" + clusterUid
	}

	r.debouncer.Start(ctx, managedBy)
	return nil
}

func (r *ResourceMonitorController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := r.createRes()
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	refs := obj.GetOwnerReferences()
	if r.resourceType == "job" && cnquery_k8s.JobOwnerReferencesFilter(refs) {
		return ctrl.Result{}, nil
	} else if r.resourceType == "pod" && cnquery_k8s.PodOwnerReferencesFilter(refs) {
		return ctrl.Result{}, nil
	} else if r.resourceType == "replicaset" && cnquery_k8s.ReplicaSetOwnerReferencesFilter(refs) {
		return ctrl.Result{}, nil
	}

	if len(r.scanApiStore.GetAll()) > 0 {
		r.debouncer.Add(fmt.Sprintf("%s:%s:%s", r.resourceType, req.Namespace, req.Name))
	}

	return ctrl.Result{}, nil
}
