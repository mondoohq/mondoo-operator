// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package operator

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"

	k8sv1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers"
	"go.mondoo.com/mondoo-operator/controllers/status"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func checkForTerminatedState(ctx context.Context, nonCacheClient client.Client, v *k8sversion.Info, logger logr.Logger) error {
	statusReport := status.NewStatusReporter(nonCacheClient, controllers.MondooClientBuilder, v)

	var err error
	config := &k8sv1alpha2.MondooOperatorConfig{}
	if err = nonCacheClient.Get(ctx, types.NamespacedName{Name: k8sv1alpha2.MondooOperatorConfigName}, config); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("MondooOperatorConfig not found, using defaults")
		} else {
			logger.Error(err, "Failed to check for MondooOpertorConfig")
			return err
		}
	}

	mondooAuditConfigs := &k8sv1alpha2.MondooAuditConfigList{}
	if err := nonCacheClient.List(ctx, mondooAuditConfigs); err != nil {
		logger.Error(err, "error listing MondooAuditConfigs")
		return err
	}

	for _, mondooAuditConfig := range mondooAuditConfigs.Items {
		mondooAuditConfigCopy := mondooAuditConfig.DeepCopy()

		podList := &corev1.PodList{}
		listOpts := &client.ListOptions{
			Namespace: mondooAuditConfig.Namespace,
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"app.kubernetes.io/name": "mondoo-operator",
			}),
		}
		if err := nonCacheClient.List(ctx, podList, listOpts); err != nil {
			logger.Error(err, "failed to list pods", "Mondoo.Namespace", mondooAuditConfig.Namespace, "Mondoo.Name", mondooAuditConfig.Name)
			return err
		}

		currentPod := k8s.GetNewestPodFromList(podList.Items)
		for _, containerStatus := range currentPod.Status.ContainerStatuses {
			if containerStatus.Name != "manager" {
				continue
			}
			stateUpdate := false
			if containerStatus.State.Terminated != nil || containerStatus.LastTerminationState.Terminated != nil {
				logger.Info("mondoo-operator was terminated before")
				// Update status
				updateOperatorConditions(&mondooAuditConfig, true, &currentPod)
				stateUpdate = true
			} else if containerStatus.RestartCount == 0 && containerStatus.State.Terminated == nil {
				logger.Info("mondoo-operator is running or starting", "state", containerStatus.State)
				updateOperatorConditions(&mondooAuditConfig, false, &corev1.Pod{})
				stateUpdate = true
			}
			if stateUpdate {
				err := mondoo.UpdateMondooAuditStatus(ctx, nonCacheClient, mondooAuditConfigCopy, &mondooAuditConfig, logger)
				if err != nil {
					logger.Error(err, "failed to update status for MondooAuditConfig")
					return err
				}
				// Report upstream before we get OOMkilled again
				err = statusReport.Report(ctx, mondooAuditConfig, *config)
				if err != nil {
					logger.Error(err, "failed to report status upstream")
					return err
				}
				break
			}
		}
	}
	return nil
}

func updateOperatorConditions(config *k8sv1alpha2.MondooAuditConfig, degradedStatus bool, pod *corev1.Pod) {
	msg := "Mondoo Operator controller is available"
	reason := "MondooOperatorAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	affectedPods := []string{}
	memoryLimit := ""
	if degradedStatus {
		msg = "Mondoo Operator controller is unavailable"
		for i, containerStatus := range pod.Status.ContainerStatuses {
			if (containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode == 137) ||
				(containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode == 137) {
				msg = "Mondoo Operator controller is unavailable due to OOM"
				affectedPods = append(affectedPods, pod.Name)
				memoryLimit = pod.Spec.Containers[i].Resources.Limits.Memory().String()
				break
			}
		}

		reason = "MondooOperatorUnavailable"
		status = corev1.ConditionTrue
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(config.Status.Conditions, k8sv1alpha2.MondooOperatorDegraded, status, reason, msg, updateCheck, affectedPods, memoryLimit)
}
