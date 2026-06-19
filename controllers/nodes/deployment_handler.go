// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package nodes

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	logutils "go.mondoo.com/mondoo-operator/pkg/utils/logger"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var logger = ctrl.Log.WithName("node-scanning")

type DeploymentHandler struct {
	KubeClient             client.Client
	Mondoo                 *v1alpha2.MondooAuditConfig
	ContainerImageResolver mondoo.ContainerImageResolver
	MondooOperatorConfig   *v1alpha2.MondooOperatorConfig
	MondooClientBuilder    func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error)
	IsOpenshift            bool
}

func (n *DeploymentHandler) log() logr.Logger {
	return logutils.WithMondooAuditConfig(logger, n.Mondoo, "node-scanning")
}

func (n *DeploymentHandler) Reconcile(ctx context.Context) (ctrl.Result, error) {
	if !n.Mondoo.Spec.Nodes.Enable {
		return ctrl.Result{}, n.down(ctx)
	}

	var syncErr error
	if len(n.Mondoo.Spec.Nodes.ScannerSets) > 0 && n.Mondoo.Spec.Nodes.Style != v1alpha2.NodeScanStyle_DaemonSet {
		syncErr = fmt.Errorf("nodes.scannerSets requires nodes.style %q", v1alpha2.NodeScanStyle_DaemonSet)
	} else {
		switch n.Mondoo.Spec.Nodes.Style {
		case v1alpha2.NodeScanStyle_CronJob:
			syncErr = n.syncCronJob(ctx)
		case v1alpha2.NodeScanStyle_Deployment, v1alpha2.NodeScanStyle_DaemonSet:
			syncErr = n.syncDaemonSet(ctx)
		}
	}

	if syncErr != nil {
		n.Mondoo.Status.Conditions = mondoo.SetMondooAuditCondition(
			n.Mondoo.Status.Conditions, v1alpha2.NodeScanningDegraded, corev1.ConditionTrue,
			"NodeScanningUnavailable", "Node scanning setup failed: "+syncErr.Error(),
			mondoo.UpdateConditionIfReasonOrMessageChange, nil, "",
		)
		// Reconciliation errors here happen before a scan workload can be trusted, so report
		// one setup-level failure instead of preserving stale per-node scan statuses.
		mondoo.ReplaceScanStatuses(n.Mondoo, v1alpha2.MondooAuditConfigScanTypeNodes, v1alpha2.MondooAuditConfigScanStatus{
			Type:    v1alpha2.MondooAuditConfigScanTypeNodes,
			Target:  "all",
			Phase:   v1alpha2.MondooAuditConfigScanPhaseFailed,
			Message: "Node scanning setup failed: " + syncErr.Error(),
		})
		return ctrl.Result{}, syncErr
	}

	// Perform garbage collection of stale node scan assets if a new successful scan has completed.
	// GC is only applicable in CronJob mode; for DaemonSet style this is a no-op since no CronJobs exist.
	clusterUid, err := k8s.GetClusterUID(ctx, n.KubeClient, n.log())
	if err != nil {
		n.log().Error(err, "Failed to get cluster's UID for node scan garbage collection")
	} else {
		n.garbageCollectIfNeeded(ctx, clusterUid)
	}

	return ctrl.Result{}, nil
}

func (n *DeploymentHandler) syncCronJob(ctx context.Context) error {
	mondooClientImage, err := n.ContainerImageResolver.CnspecImage(
		n.Mondoo.Spec.Scanner.Image.Name, n.Mondoo.Spec.Scanner.Image.Tag, n.Mondoo.Spec.Scanner.Image.Digest, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		n.log().Error(err, "Failed to resolve mondoo-client container image")
		return err
	}

	clusterUid, err := k8s.GetClusterUID(ctx, n.KubeClient, n.log())
	if err != nil {
		n.log().Error(err, "Failed to get cluster's UID")
		return err
	}

	nodes := &corev1.NodeList{}
	nodeListOpts, err := nodeListOptions(n.Mondoo)
	if err != nil {
		return err
	}
	if err := n.KubeClient.List(ctx, nodes, nodeListOpts...); err != nil {
		n.log().Error(err, "Failed to list cluster nodes")
		return err
	}

	if err := n.cleanupOrphanedDaemonSets(ctx, map[string]struct{}{}); err != nil {
		n.log().Error(err, "Failed to clean up node scanning DaemonSets")
		return err
	}
	if err := n.cleanupLegacyPerNodeResources(ctx); err != nil {
		return err
	}

	if err := n.syncConfigMap(ctx, clusterUid); err != nil {
		return err
	}

	// Collected across all nodes so the warning is emitted once per reconcile
	// instead of once per affected node.
	var (
		unsafeMemoryNodes       []string
		unsafeMemoryLimit       resource.Quantity
		unsafeMemoryAllocatable resource.Quantity
		haveUnsafeMemory        bool
	)

	// Create/update CronJobs for nodes
	for _, node := range nodes.Items {

		// Check the limit the node actually gets: a matching node scan profile overrides
		// spec.nodes.resources, so the warning has to follow the same resolution as CronJob().
		nodeResources := ProfileResources(n.Mondoo.Spec.Nodes, ProfileForNode(n.Mondoo.Spec.Nodes, node))
		if limit, allocatable, unsafe := UnsafeNodeScanMemoryLimit(
			k8s.ResourcesRequirementsWithDefaults(nodeResources, k8s.DefaultNodeScanningResources),
			node,
		); unsafe {
			unsafeMemoryNodes = append(unsafeMemoryNodes, node.Name)
			// Report the node with the least allocatable memory, which is the
			// worst case for the configured limit.
			if !haveUnsafeMemory || allocatable.Cmp(unsafeMemoryAllocatable) < 0 {
				unsafeMemoryLimit = limit
				unsafeMemoryAllocatable = allocatable
				haveUnsafeMemory = true
			}
		}

		desired := CronJob(mondooClientImage, node, n.Mondoo, n.IsOpenshift, *n.MondooOperatorConfig)
		cronJob := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
		op, err := k8s.CreateOrUpdate(ctx, n.KubeClient, cronJob, n.Mondoo, n.log(), func() error {
			k8s.UpdateCronJobFields(cronJob, desired)
			return nil
		})
		if err != nil {
			return err
		}

		switch op {
		case controllerutil.OperationResultCreated:
			if err := mondoo.UpdateMondooAuditConfig(ctx, n.KubeClient, n.Mondoo, n.log()); err != nil {
				n.log().Error(err, "Failed to update MondooAuditConfig", "namespace", n.Mondoo.Namespace, "name", n.Mondoo.Name)
				return err
			}
		case controllerutil.OperationResultUpdated:
			// Remove completed Jobs so they don't linger with stale config
			if err := k8s.DeleteCompletedJobs(ctx, n.KubeClient, n.Mondoo.Namespace, NodeScanningLabels(*n.Mondoo), n.log()); err != nil {
				n.log().Error(err, "Failed to clean up completed Jobs after CronJob update")
				return err
			}
		}
	}

	if haveUnsafeMemory {
		n.log().Info(
			nodeScanMemoryLimitWarning(unsafeMemoryLimit, unsafeMemoryAllocatable),
			"nodes", strings.Join(unsafeMemoryNodes, ","),
		)
	}

	// Delete dangling CronJobs for nodes that have been deleted from the cluster.
	if err := n.cleanupCronJobsForDeletedNodes(ctx, *nodes); err != nil {
		return err
	}

	// List the CronJobs again after they have been synced.
	cronJobs, err := n.getCronJobsForAuditConfig(ctx)
	if err != nil {
		return err
	}

	// Get Pods for this CronJob
	pods := &corev1.PodList{}
	if len(cronJobs) > 0 {
		opts := &client.ListOptions{
			Namespace:     n.Mondoo.Namespace,
			LabelSelector: labels.SelectorFromSet(NodeScanningLabels(*n.Mondoo)),
		}
		err = n.KubeClient.List(ctx, pods, opts)
		if err != nil {
			n.log().Error(err, "Failed to list Pods for Node Scanning")
			return err
		}
	}

	updateNodeConditions(n.Mondoo, !k8s.AreCronJobsSuccessful(cronJobs), pods)
	n.updateCronJobScanStatuses(cronJobs, nodes.Items)

	// Clean up any leftover GC CronJobs from previous versions
	gcCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: GarbageCollectCronJobName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, gcCronJob); err != nil {
		n.log().Error(err, "Failed to clean up node garbage collect CronJob", "namespace", gcCronJob.Namespace, "name", gcCronJob.Name)
		return err
	}

	return nil
}

func (n *DeploymentHandler) syncDaemonSet(ctx context.Context) error {
	if err := validateScannerSets("nodes.scannerSets", n.Mondoo.Spec.Nodes.ScannerSets); err != nil {
		return err
	}
	mondooClientImage, err := n.ContainerImageResolver.CnspecImage(
		n.Mondoo.Spec.Scanner.Image.Name, n.Mondoo.Spec.Scanner.Image.Tag, n.Mondoo.Spec.Scanner.Image.Digest, n.MondooOperatorConfig.Spec.SkipContainerResolution)
	if err != nil {
		n.log().Error(err, "Failed to resolve mondoo-client container image")
		return err
	}

	clusterUid, err := k8s.GetClusterUID(ctx, n.KubeClient, n.log())
	if err != nil {
		n.log().Error(err, "Failed to get cluster's UID")
		return err
	}

	nodes := &corev1.NodeList{}
	nodeListOpts, err := nodeListOptions(n.Mondoo)
	if err != nil {
		return err
	}
	if err := n.KubeClient.List(ctx, nodes, nodeListOpts...); err != nil {
		n.log().Error(err, "Failed to list cluster nodes")
		return err
	}

	if err := n.cleanupCronJobsForDeletedNodes(ctx, *nodes); err != nil {
		return err
	}
	if err := n.cleanupLegacyPerNodeResources(ctx); err != nil {
		return err
	}

	if err := n.syncConfigMap(ctx, clusterUid); err != nil {
		return err
	}

	// Create/update Deployments for nodes
	for _, node := range nodes.Items {
		// Delete CronJob if it exists
		cronJob := &batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{Name: CronJobName(n.Mondoo.Name, node.Name), Namespace: n.Mondoo.Namespace},
		}
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, cronJob); err != nil {
			n.log().Error(err, "Failed to clean up node scanning CronJob", "namespace", cronJob.Namespace, "name", cronJob.Name)
			return err
		}

		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: ConfigMapNameWithNode(n.Mondoo.Name, node.Name), Namespace: n.Mondoo.Namespace}}
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, cm); err != nil {
			n.log().Error(err, "Failed to clean up old ConfigMap for node scanning", "namespace", cm.Namespace, "name", cm.Name)
			return err
		}

		if n.Mondoo.Spec.Nodes.Style == v1alpha2.NodeScanStyle_Deployment {
			dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: DeploymentName(n.Mondoo.Name, node.Name), Namespace: n.Mondoo.Namespace}}
			if err := k8s.DeleteIfExists(ctx, n.KubeClient, dep); err != nil {
				n.log().Error(err, "Failed to clean up node scanning Deployment", "namespace", dep.Namespace, "name", dep.Name)
			}
		}
	}

	var inheritedTolerations []corev1.Toleration
	if len(n.Mondoo.Spec.Nodes.ScannerSets) == 0 {
		tolerations := make(map[corev1.Toleration]struct{})
		for _, node := range nodes.Items {
			for _, toleration := range k8s.TaintsToTolerations(node.Spec.Taints) {
				tolerations[toleration] = struct{}{}
			}
		}
		// maps.Keys iterates in random order, so sort to avoid a spurious update on every reconcile.
		inheritedTolerations = slices.Collect(maps.Keys(tolerations))
		k8s.SortTolerations(inheritedTolerations)
	}

	desiredSets := n.desiredDaemonSets(mondooClientImage, inheritedTolerations)
	expectedNames := map[string]struct{}{}
	degraded := false
	created := false
	daemonSets := make([]appsv1.DaemonSet, 0, len(desiredSets))
	for _, desired := range desiredSets {
		expectedNames[desired.Name] = struct{}{}
		ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
		op, err := k8s.CreateOrUpdate(ctx, n.KubeClient, ds, n.Mondoo, n.log(), func() error {
			k8s.UpdateDaemonSetFields(ds, desired)
			return nil
		})
		if err != nil {
			return err
		}
		created = created || op == controllerutil.OperationResultCreated

		if err := n.KubeClient.Get(ctx, client.ObjectKeyFromObject(ds), ds); err != nil {
			n.log().Error(err, "Failed to get DaemonSet", "namespace", ds.Namespace, "name", ds.Name)
			return err
		}
		degraded = degraded || daemonSetDegraded(ds)
		daemonSets = append(daemonSets, *ds)
	}
	if created {
		if err := mondoo.UpdateMondooAuditConfig(ctx, n.KubeClient, n.Mondoo, n.log()); err != nil {
			n.log().Error(err, "Failed to update MondooAuditConfig", "namespace", n.Mondoo.Namespace, "name", n.Mondoo.Name)
			return err
		}
	}
	if err := n.cleanupOrphanedDaemonSets(ctx, expectedNames); err != nil {
		return err
	}

	// Get Pods for these Deployments
	pods := &corev1.PodList{}
	opts := &client.ListOptions{
		Namespace:     n.Mondoo.Namespace,
		LabelSelector: labels.SelectorFromSet(NodeScanningLabels(*n.Mondoo)),
	}
	err = n.KubeClient.List(ctx, pods, opts)
	if err != nil {
		n.log().Error(err, "Failed to list Pods for Node Scanning")
		return err
	}

	updateNodeConditions(n.Mondoo, degraded, pods)
	n.updateDaemonSetScanStatus(daemonSets)

	// Clean up any leftover GC CronJobs from previous versions
	gcCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: GarbageCollectCronJobName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, gcCronJob); err != nil {
		n.log().Error(err, "Failed to clean up node garbage collect CronJob", "namespace", gcCronJob.Namespace, "name", gcCronJob.Name)
		return err
	}

	return nil
}

func nodeListOptions(m *v1alpha2.MondooAuditConfig) ([]client.ListOption, error) {
	if m.Spec.Nodes.LabelSelector == nil {
		return nil, nil
	}
	selector, err := metav1.LabelSelectorAsSelector(m.Spec.Nodes.LabelSelector)
	if err != nil {
		return nil, err
	}
	return []client.ListOption{client.MatchingLabelsSelector{Selector: selector}}, nil
}

func (n *DeploymentHandler) desiredDaemonSets(image string, inheritedTolerations []corev1.Toleration) []*appsv1.DaemonSet {
	if len(n.Mondoo.Spec.Nodes.ScannerSets) == 0 {
		return []*appsv1.DaemonSet{
			DaemonSet(*n.Mondoo, n.IsOpenshift, image, *n.MondooOperatorConfig, inheritedTolerations),
		}
	}
	out := make([]*appsv1.DaemonSet, 0, len(n.Mondoo.Spec.Nodes.ScannerSets))
	for _, set := range n.Mondoo.Spec.Nodes.ScannerSets {
		out = append(out, DaemonSetForScannerSet(*n.Mondoo, n.IsOpenshift, image, *n.MondooOperatorConfig, inheritedTolerations, set))
	}
	return out
}

func daemonSetDegraded(ds *appsv1.DaemonSet) bool {
	if ds.Status.ObservedGeneration < ds.Generation {
		return false
	}
	if ds.Status.DesiredNumberScheduled == 0 {
		return true
	}
	return ds.Status.NumberUnavailable > 0 || ds.Status.NumberReady < ds.Status.DesiredNumberScheduled
}

func (n *DeploymentHandler) syncConfigMap(ctx context.Context, clusterUid string) error {
	integrationMrn, err := k8s.TryGetIntegrationMrnForAuditConfig(ctx, n.KubeClient, *n.Mondoo)
	if err != nil {
		n.log().Error(err, "failed to retrieve IntegrationMRN")
		return err
	}

	desired, err := ConfigMap(integrationMrn, clusterUid, *n.Mondoo)
	if err != nil {
		n.log().Error(err, "failed to generate ConfigMap")
		return err
	}

	obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	if _, err := k8s.CreateOrUpdate(ctx, n.KubeClient, obj, n.Mondoo, n.log(), func() error {
		obj.Labels = desired.Labels
		obj.Data = desired.Data
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// cleanupLegacyPerNodeResources removes the per-node Deployments and ConfigMaps that
// older operator versions created. Both lists go through the cached client and are
// filtered by the node scanning labels, so they are cheap once nothing is left.
// TODO: drop this migration path once releases that created those resources are out of support.
func (n *DeploymentHandler) cleanupLegacyPerNodeResources(ctx context.Context) error {
	listOpts := &client.ListOptions{
		Namespace:     n.Mondoo.Namespace,
		LabelSelector: labels.SelectorFromSet(NodeScanningLabels(*n.Mondoo)),
	}

	deployments := &appsv1.DeploymentList{}
	if err := n.KubeClient.List(ctx, deployments, listOpts); err != nil {
		logger.Error(err, "Failed to list legacy node scanning Deployments", "namespace", n.Mondoo.Namespace)
		return err
	}
	for i := range deployments.Items {
		deployment := &deployments.Items[i]
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, deployment); err != nil {
			logger.Error(err, "Failed to clean up legacy node scanning Deployment", "namespace", deployment.Namespace, "name", deployment.Name)
			return err
		}
	}

	configMaps := &corev1.ConfigMapList{}
	if err := n.KubeClient.List(ctx, configMaps, listOpts); err != nil {
		logger.Error(err, "Failed to list legacy node scanning ConfigMaps", "namespace", n.Mondoo.Namespace)
		return err
	}
	for i := range configMaps.Items {
		configMap := &configMaps.Items[i]
		if configMap.Name == ConfigMapName(n.Mondoo.Name) {
			continue
		}
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, configMap); err != nil {
			logger.Error(err, "Failed to clean up legacy node scanning ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
			return err
		}
	}

	return nil
}

// cleanupCronJobsForDeletedNodes deletes dangling CronJobs for nodes that have been deleted from the cluster.
func (n *DeploymentHandler) cleanupCronJobsForDeletedNodes(ctx context.Context, currentNodes corev1.NodeList) error {
	cronJobs, err := n.getCronJobsForAuditConfig(ctx)
	if err != nil {
		return err
	}

	for _, c := range cronJobs {
		// Check if the node for that CronJob is still present in the cluster.
		found := false
		for _, node := range currentNodes.Items {
			if CronJobName(n.Mondoo.Name, node.Name) == c.Name {
				found = true
				break
			}
		}

		// If the node is still there, there is nothing to update.
		if found {
			continue
		}

		// If the node for the CronJob has been deleted from the cluster, the CronJob needs to be deleted.
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, &c); err != nil {
			n.log().Error(err, "Failed to deleted CronJob", "namespace", c.Namespace, "name", c.Name)
			return err
		}
		n.log().Info("Deleted CronJob", "namespace", c.Namespace, "name", c.Name)
	}
	return nil
}

func (n *DeploymentHandler) getCronJobsForAuditConfig(ctx context.Context) ([]batchv1.CronJob, error) {
	cronJobs := &batchv1.CronJobList{}
	cronJobLabels := NodeScanningLabels(*n.Mondoo)

	// Lists only the CronJobs in the namespace of the MondooAuditConfig and only the ones that exactly match our labels.
	listOpts := &client.ListOptions{Namespace: n.Mondoo.Namespace, LabelSelector: labels.SelectorFromSet(cronJobLabels)}
	if err := n.KubeClient.List(ctx, cronJobs, listOpts); err != nil {
		n.log().Error(err, "Failed to list CronJobs in namespace", "namespace", n.Mondoo.Namespace)
		return nil, err
	}
	return cronJobs.Items, nil
}

func (n *DeploymentHandler) down(ctx context.Context) error {
	nodes := &corev1.NodeList{}
	if err := n.KubeClient.List(ctx, nodes); err != nil {
		n.log().Error(err, "Failed to list cluster nodes")
		return err
	}

	for _, node := range nodes.Items {
		cronJob := &batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{Name: CronJobName(n.Mondoo.Name, node.Name), Namespace: n.Mondoo.Namespace},
		}
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, cronJob); err != nil {
			n.log().Error(err, "Failed to clean up node scanning CronJob", "namespace", cronJob.Namespace, "name", cronJob.Name)
			return err
		}
	}

	if err := n.cleanupOrphanedDaemonSets(ctx, map[string]struct{}{}); err != nil {
		n.log().Error(err, "Failed to clean up node scanning DaemonSets")
		return err
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: ConfigMapName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, configMap); err != nil {
		n.log().Error(err, "Failed to clean up inventory ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
		return err
	}

	gcCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: GarbageCollectCronJobName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace},
	}
	if err := k8s.DeleteIfExists(ctx, n.KubeClient, gcCronJob); err != nil {
		n.log().Error(err, "Failed to clean up node garbage collect CronJob", "namespace", gcCronJob.Namespace, "name", gcCronJob.Name)
		return err
	}

	// Update any remnant conditions
	updateNodeConditions(n.Mondoo, false, &corev1.PodList{})
	mondoo.ReplaceScanStatuses(
		n.Mondoo,
		v1alpha2.MondooAuditConfigScanTypeNodes,
		mondoo.DisabledScanStatus(v1alpha2.MondooAuditConfigScanTypeNodes, "all"),
	)

	return nil
}

func (n *DeploymentHandler) updateCronJobScanStatuses(cronJobs []batchv1.CronJob, nodes []corev1.Node) {
	cronJobsByName := make(map[string]batchv1.CronJob, len(cronJobs))
	for _, cronJob := range cronJobs {
		cronJobsByName[cronJob.Name] = cronJob
	}

	nodeNames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}
	sort.Strings(nodeNames)

	statuses := make([]v1alpha2.MondooAuditConfigScanStatus, 0, len(nodeNames))
	for _, nodeName := range nodeNames {
		cronJobName := CronJobName(n.Mondoo.Name, nodeName)
		cronJob, ok := cronJobsByName[cronJobName]
		if !ok {
			statuses = append(statuses, v1alpha2.MondooAuditConfigScanStatus{
				Type:    v1alpha2.MondooAuditConfigScanTypeNodes,
				Target:  nodeName,
				Phase:   v1alpha2.MondooAuditConfigScanPhasePending,
				Message: "Scan CronJob has not been created yet",
			})
			continue
		}
		statuses = append(statuses, mondoo.ScanStatusFromCronJob(v1alpha2.MondooAuditConfigScanTypeNodes, nodeName, cronJob))
	}

	mondoo.ReplaceScanStatuses(n.Mondoo, v1alpha2.MondooAuditConfigScanTypeNodes, statuses...)
}

func (n *DeploymentHandler) updateDaemonSetScanStatus(daemonSets []appsv1.DaemonSet) {
	status := v1alpha2.MondooAuditConfigScanStatus{
		Type:    v1alpha2.MondooAuditConfigScanTypeNodes,
		Target:  "all",
		Phase:   v1alpha2.MondooAuditConfigScanPhaseRunning,
		Message: "Node scanning DaemonSet is scheduled",
	}
	if len(daemonSets) == 0 {
		status.Phase = v1alpha2.MondooAuditConfigScanPhasePending
		status.Message = "Node scanning DaemonSet has not been created yet"
		mondoo.ReplaceScanStatuses(n.Mondoo, v1alpha2.MondooAuditConfigScanTypeNodes, status)
		return
	}
	for i := range daemonSets {
		ds := daemonSets[i]
		if ds.Status.DesiredNumberScheduled == 0 {
			status.Phase = v1alpha2.MondooAuditConfigScanPhasePending
			status.Message = "Node scanning DaemonSet has not reported desired nodes yet"
			break
		} else if ds.Status.CurrentNumberScheduled < ds.Status.DesiredNumberScheduled {
			status.Phase = v1alpha2.MondooAuditConfigScanPhaseFailed
			status.Message = "Node scanning DaemonSet is not scheduled on all desired nodes"
			break
		}
	}
	mondoo.ReplaceScanStatuses(n.Mondoo, v1alpha2.MondooAuditConfigScanTypeNodes, status)
}

func (n *DeploymentHandler) cleanupOrphanedDaemonSets(ctx context.Context, expectedNames map[string]struct{}) error {
	legacy := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: DaemonSetName(n.Mondoo.Name), Namespace: n.Mondoo.Namespace}}
	if _, ok := expectedNames[legacy.Name]; !ok {
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, legacy); err != nil {
			n.log().Error(err, "Failed to clean up legacy node scanning DaemonSet", "namespace", legacy.Namespace, "name", legacy.Name)
			return err
		}
	}

	list := &appsv1.DaemonSetList{}
	if err := n.KubeClient.List(ctx, list, &client.ListOptions{
		Namespace:     n.Mondoo.Namespace,
		LabelSelector: labels.SelectorFromSet(NodeScanningLabels(*n.Mondoo)),
	}); err != nil {
		n.log().Error(err, "Failed to list node scanning DaemonSets")
		return err
	}
	for i := range list.Items {
		ds := &list.Items[i]
		if _, ok := expectedNames[ds.Name]; ok {
			continue
		}
		if err := k8s.DeleteIfExists(ctx, n.KubeClient, ds); err != nil {
			n.log().Error(err, "Failed to clean up node scanning DaemonSet", "namespace", ds.Namespace, "name", ds.Name)
			return err
		}
	}
	return nil
}

func validateScannerSets(path string, sets []v1alpha2.NodeScannerSet) error {
	seen := map[string]struct{}{}
	for i, set := range sets {
		if set.Name == "" {
			return fmt.Errorf("%s[%d].name is required", path, i)
		}
		if errs := k8svalidation.IsDNS1123Label(set.Name); len(errs) > 0 {
			return fmt.Errorf("%s[%s].name must be a valid DNS label: %s", path, set.Name, strings.Join(errs, "; "))
		}
		if _, ok := seen[set.Name]; ok {
			return fmt.Errorf("%s[%s].name must be unique", path, set.Name)
		}
		seen[set.Name] = struct{}{}
	}
	return nil
}

// garbageCollectIfNeeded checks whether a new successful node scan has completed since the last GC run,
// and if so, performs garbage collection of stale node assets via the Mondoo API.
func (n *DeploymentHandler) garbageCollectIfNeeded(ctx context.Context, clusterUid string) {
	cronJobs, err := n.getCronJobsForAuditConfig(ctx)
	if err != nil {
		n.log().Error(err, "Failed to list CronJobs for node scan garbage collection")
		return
	}

	// Find the latest lastSuccessfulTime across all node scan CronJobs
	var latestSuccess *metav1.Time
	for i := range cronJobs {
		t := cronJobs[i].Status.LastSuccessfulTime
		if t != nil && (latestSuccess == nil || t.After(latestSuccess.Time)) {
			latestSuccess = t
		}
	}

	if latestSuccess == nil {
		// No successful scans yet
		return
	}

	// Skip if we already ran GC for this (or a newer) successful scan
	if n.Mondoo.Status.LastNodeScanGarbageCollectionTime != nil &&
		!latestSuccess.After(n.Mondoo.Status.LastNodeScanGarbageCollectionTime.Time) {
		return
	}

	managedBy := mondoo.ManagedByNodesLabel(clusterUid)
	if err := n.performGarbageCollection(ctx, managedBy); err != nil {
		n.log().Error(err, "Failed to perform garbage collection of node scan assets")
	}

	// Always update the timestamp so we don't retry until the next new successful scan.
	// GC failure is non-critical — stale assets will be cleaned up on the next attempt.
	now := metav1.Now()
	n.Mondoo.Status.LastNodeScanGarbageCollectionTime = &now
}

// performGarbageCollection calls the Mondoo API to delete stale node scan assets.
func (n *DeploymentHandler) performGarbageCollection(ctx context.Context, managedBy string) error {
	// Node assets are scanned via the filesystem/OS provider and have no PlatformRuntime set
	// (unlike k8s resource assets which have PlatformRuntime "k8s-cluster").
	// Omitting PlatformRuntime so the filter matches node assets.
	req := &mondooclient.GarbageCollectAssetsRequest{
		ManagedBy: managedBy,
		Labels:    map[string]string{"k8s.mondoo.com/kind": "node"},
		DateFilter: &mondooclient.DateFilter{
			Timestamp:  time.Now().Add(-mondoo.GCOlderThan(n.Mondoo.Spec.Nodes.Schedule)).Format(time.RFC3339),
			Comparison: mondooclient.Comparison_LESS_THAN,
			Field:      mondooclient.DateFilterField_FILTER_LAST_UPDATED,
		},
	}

	if err := mondoo.GarbageCollectAssets(ctx, n.KubeClient, n.Mondoo, n.MondooOperatorConfig, n.MondooClientBuilder, req, n.log()); err != nil {
		return err
	}

	n.log().Info("Successfully performed garbage collection of node scan assets")
	return nil
}
