// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package utils

import (
	"fmt"
	"os"
	"time"

	mondoov2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	MondooClientSecret   = "mondoo-client"
	MondooTokenSecret    = "mondoo-token"
	CnspecImageTagEnvVar = "CNSPEC_IMAGE_TAG"
)

var cnspecImageTag = ""

func init() {
	imageTag, ok := os.LookupEnv(CnspecImageTagEnvVar)
	if ok {
		cnspecImageTag = imageTag
	}
}

// DefaultAuditConfigMinimal returns a new Mondoo audit config with minimal default settings to
// make sure a test passes (e.g. setting the correct secret name). Values which have defaults are not set.
// This means that using this function in unit tests might result in strange behavior. For unit tests use
// DefaultAuditConfig instead.
func DefaultAuditConfigMinimal(ns string, workloads, containers, nodes bool) mondoov2.MondooAuditConfig {
	now := time.Now()
	// The cron schedule only uses the minute field, so the real buffer is
	// (targetMinuteStart - now). With a 2m offset the minimum buffer is ~61s,
	// which safely covers leader election (~45s) plus CronJob creation time,
	// while keeping the worst-case trigger (~120s) within the retry window.
	startScan := now.Add(2 * time.Minute)
	schedule := fmt.Sprintf("%d * * * *", startScan.Minute())
	auditConfig := mondoov2.MondooAuditConfig{
		TypeMeta: v1.TypeMeta{
			APIVersion: "k8s.mondoo.com/v1alpha2",
			Kind:       "MondooAuditConfig",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "mondoo-client",
			Namespace: ns,
		},
		Spec: mondoov2.MondooAuditConfigSpec{
			ConsoleIntegration:   mondoov2.ConsoleIntegration{Enable: true},
			MondooCredsSecretRef: corev1.LocalObjectReference{Name: MondooClientSecret},
			MondooTokenSecretRef: corev1.LocalObjectReference{Name: MondooTokenSecret},
			KubernetesResources: mondoov2.KubernetesResources{
				Enable:   workloads,
				Schedule: schedule,
			},
			Containers: mondoov2.Containers{
				Enable:   containers,
				Schedule: schedule,
			},
			Nodes: mondoov2.Nodes{
				Enable:   nodes,
				Schedule: schedule,
				Style:    mondoov2.NodeScanStyle_CronJob,
			},
		},
	}

	// cnspec doesn't get edge releases at the moment, so we cannot test that
	if cnspecImageTag != "" {
		auditConfig.Spec.Scanner.Image.Tag = cnspecImageTag
		zap.S().Infof("Using image %s:%s for mondoo-client", mondoo.CnspecImage, cnspecImageTag)
	}

	return auditConfig
}

// DefaultAuditConfig returns a new Mondoo audit config with some default settings to
// make sure a tests passes (e.g. setting the correct secret name).
func DefaultAuditConfig(ns string, workloads, containers, nodes bool) mondoov2.MondooAuditConfig {
	return mondoov2.MondooAuditConfig{
		TypeMeta: v1.TypeMeta{
			APIVersion: "k8s.mondoo.com/v1alpha2",
			Kind:       "MondooAuditConfig",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "mondoo-client",
			Namespace: ns,
		},
		Spec: mondoov2.MondooAuditConfigSpec{
			MondooCredsSecretRef: corev1.LocalObjectReference{Name: MondooClientSecret},
			KubernetesResources:  mondoov2.KubernetesResources{Enable: workloads, Schedule: "0 * * * *"},
			Containers:           mondoov2.Containers{Enable: containers, Schedule: "0 0 * * *"},
			Nodes:                mondoov2.Nodes{Enable: nodes, Schedule: "0 * * * *", Style: mondoov2.NodeScanStyle_CronJob},
			Scanner: mondoov2.Scanner{
				Image:    mondoov2.Image{Name: "test", Tag: "latest"},
				Replicas: ptr.To(int32(1)),
			},
		},
	}
}
