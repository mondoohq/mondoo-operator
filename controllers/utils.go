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

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mondoov1alpha1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
)

const (
	mondooImage         = "docker.io/mondoo/client"
	mondooTag           = "latest"
	mondooOperatorImage = "ghcr.io/mondoohq/mondoo-operator"
	mondooOperatorTag   = "latest"
)

func resolveMondooImage(log logr.Logger, userImageName, userImageTag string, resolveImage bool) (string, error) {
	useImage := mondooImage
	useTag := mondooTag
	if userImageName != "" {
		useImage = userImageName
	}
	if userImageTag != "" {
		useTag = userImageTag
	}
	mondooContainer := useImage + ":" + useTag
	imageUrl, err := skipResolveImage(resolveImage, log, mondooContainer)
	if err != nil {
		log.Error(err, "Failed to skip resolving image")
		return imageUrl, err
	}
	return imageUrl, nil

}

func resolveMondooOperatorImage(log logr.Logger, userImageName, userImageTag string, resolveImage bool) (string, error) {
	useImage := mondooOperatorImage
	useTag := mondooOperatorTag
	if userImageName != "" {
		useImage = userImageName
	}
	if userImageTag != "" {
		useTag = userImageTag
	}
	mondooContainer := useImage + ":" + useTag

	imageUrl, err := skipResolveImage(resolveImage, log, mondooContainer)
	if err != nil {
		log.Error(err, "Failed to skip resolving image")
		return imageUrl, err
	}
	return imageUrl, nil
}

func skipResolveImage(resolveImage bool, log logr.Logger, mondooContainer string) (string, error) {
	if !resolveImage {
		imageUrl, err := parseReference(log, mondooContainer)
		if err != nil {
			log.Error(err, "Failed to parse reference")
			return "", err
		}
		return imageUrl, nil
	}
	return mondooContainer, nil
}

func parseReference(log logr.Logger, container string) (string, error) {
	ref, err := name.ParseReference(container)
	if err != nil {
		log.Error(err, "Failed to parse container reference")
		return "", err
	}

	desc, err := remote.Get(ref)
	if err != nil {
		log.Error(err, "Failed to get container reference")
		return "", err
	}
	imgDigest := desc.Digest.String()
	repoName := ref.Context().Name()
	imageUrl := repoName + "@" + imgDigest

	return imageUrl, nil
}

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
func FindMondooOperatorConfigCondition(conditions []mondoov1alpha1.MondooOperatorConfigCondition, conditionType mondoov1alpha1.MondooOperatorConfigConditionType) *mondoov1alpha1.MondooOperatorConfigCondition {
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
	conditions []mondoov1alpha1.MondooOperatorConfigCondition,
	conditionType mondoov1alpha1.MondooOperatorConfigConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) []mondoov1alpha1.MondooOperatorConfigCondition {
	now := metav1.Now()
	existingCondition := FindMondooOperatorConfigCondition(conditions, conditionType)
	if existingCondition == nil {
		if status == corev1.ConditionTrue {
			conditions = append(
				conditions,
				mondoov1alpha1.MondooOperatorConfigCondition{
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

func UpdateMondooOperatorConfigStatus(ctx context.Context, client client.Client, origMOC, newMOC *mondoov1alpha1.MondooOperatorConfig, log logr.Logger) error {
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
