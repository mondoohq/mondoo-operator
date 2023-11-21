/*
Copyright 2017 The Kubernetes Authors.
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

package mondoo

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
)

// UpdateConditionCheck tests whether a condition should be updated from the
// old condition to the new condition. Returns true if the condition should
// be updated.
type UpdateConditionCheck func(oldReason, oldMessage, newReason, newMessage string) bool

// UpdateConditionAlways returns true. The condition will always be updated.
func UpdateConditionAlways(_, _, _, _ string) bool {
	return true
}

// UpdateConditionIfReasonOrMessageChange returns true if there is a change
// in the reason or the message of the condition.
func UpdateConditionIfReasonOrMessageChange(oldReason, oldMessage, newReason, newMessage string) bool {
	return oldReason != newReason ||
		oldMessage != newMessage
}

// UpdateConditionNever return false. The condition will never be updated,
// unless there is a change in the status of the condition.
func UpdateConditionNever(_, _, _, _ string) bool {
	return false
}

// FindMondooOperatorConfigCondition iterates all conditions on a MondooOperatorConfig looking for the
// specified condition type. If none exists nil will be returned.
func FindMondooOperatorConfigCondition(conditions []mondoov1alpha2.MondooOperatorConfigCondition, conditionType mondoov1alpha2.MondooOperatorConfigConditionType) *mondoov1alpha2.MondooOperatorConfigCondition {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

func shouldUpdateCondition(
	oldStatus corev1.ConditionStatus, oldReason, oldMessage string,
	newStatus corev1.ConditionStatus, newReason, newMessage string,
	updateConditionCheck UpdateConditionCheck,
) bool {
	if oldStatus != newStatus {
		return true
	}
	return updateConditionCheck(oldReason, oldMessage, newReason, newMessage)
}

// SetMondooOperatorConfigCondition sets the condition for the MondooOperatorConfig and returns the new slice of conditions.
// If the MondooAuditCOnfi does not already have a condition with the specified type,
// a condition will be added to the slice if and only if the specified
// status is True.
// If the MondooAuditConfig does already have a condition with the specified type,
// the condition will be updated if either of the following are true.
// 1) Requested status is different than existing status.
// 2) The updateConditionCheck function returns true.
func SetMondooOperatorConfigCondition(
	conditions []mondoov1alpha2.MondooOperatorConfigCondition,
	conditionType mondoov1alpha2.MondooOperatorConfigConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) []mondoov1alpha2.MondooOperatorConfigCondition {
	now := metav1.Now()
	existingCondition := FindMondooOperatorConfigCondition(conditions, conditionType)
	if existingCondition == nil {
		if status == corev1.ConditionTrue {
			conditions = append(
				conditions,
				mondoov1alpha2.MondooOperatorConfigCondition{
					Type:               conditionType,
					Status:             status,
					Reason:             reason,
					Message:            message,
					LastTransitionTime: now,
					LastUpdateTime:     now,
				},
			)
		}
	} else {
		if shouldUpdateCondition(
			existingCondition.Status, existingCondition.Reason, existingCondition.Message,
			status, reason, message,
			updateConditionCheck,
		) {
			if existingCondition.Status != status {
				existingCondition.LastTransitionTime = now
			}
			existingCondition.Status = status
			existingCondition.Reason = reason
			existingCondition.Message = message
			existingCondition.LastUpdateTime = now
		}
	}
	return conditions
}

func UpdateMondooOperatorConfigStatus(ctx context.Context, client client.Client, origMOC, newMOC *mondoov1alpha2.MondooOperatorConfig, log logr.Logger) error {
	if !reflect.DeepEqual(origMOC.Status, newMOC.Status) {
		log.Info("status has changed, updating")
		err := client.Status().Update(ctx, newMOC)
		if err != nil {
			log.Error(err, "failed to update status")
			return err
		}
	}
	return nil
}

func FindMondooAuditConditions(conditions []mondoov1alpha2.MondooAuditConfigCondition, conditionType mondoov1alpha2.MondooAuditConfigConditionType) *mondoov1alpha2.MondooAuditConfigCondition {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

func SetMondooAuditCondition(
	conditions []mondoov1alpha2.MondooAuditConfigCondition,
	conditionType mondoov1alpha2.MondooAuditConfigConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
	affectedPods []string,
	memoryLimit string,
) []mondoov1alpha2.MondooAuditConfigCondition {
	now := metav1.Now()
	existingCondition := FindMondooAuditConditions(conditions, conditionType)
	if existingCondition == nil {
		conditions = append(
			conditions,
			mondoov1alpha2.MondooAuditConfigCondition{
				Type:               conditionType,
				Status:             status,
				Reason:             reason,
				Message:            message,
				LastTransitionTime: now,
				LastUpdateTime:     now,
				AffectedPods:       affectedPods,
				MemoryLimit:        memoryLimit,
			},
		)
	} else {
		if shouldUpdateCondition(
			existingCondition.Status, existingCondition.Reason, existingCondition.Message,
			status, reason, message,
			updateConditionCheck,
		) {
			if existingCondition.Status != status {
				existingCondition.LastTransitionTime = now
			}
			existingCondition.Status = status
			existingCondition.Reason = reason
			existingCondition.Message = message
			existingCondition.LastUpdateTime = now
			existingCondition.AffectedPods = affectedPods
			existingCondition.MemoryLimit = memoryLimit
		}
	}
	return conditions
}

func UpdateMondooAuditStatus(ctx context.Context, client client.Client, origMOC, newMOC *mondoov1alpha2.MondooAuditConfig, log logr.Logger) error {
	if !reflect.DeepEqual(origMOC.Status, newMOC.Status) {
		log.Info("status has changed, updating")
		err := client.Status().Update(ctx, newMOC)
		if err != nil {
			log.Error(err, "failed to update status")
			return err
		}
	}
	return nil
}

func UpdateMondooAuditConfig(ctx context.Context, k8sClient client.Client, newMAC *mondoov1alpha2.MondooAuditConfig, log logr.Logger) error {
	currentMAC := &mondoov1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      newMAC.Name,
			Namespace: newMAC.Namespace,
		},
	}
	err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(currentMAC), currentMAC)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(currentMAC.Spec, newMAC.Spec) {
		log.Info("status has changed, updating")
		err := k8sClient.Update(ctx, newMAC)
		if err != nil {
			log.Error(err, "failed to update mondooauditconfig")
			return err
		}
	}
	return nil
}
