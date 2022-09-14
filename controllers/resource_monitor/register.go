/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package resource_monitor

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var resourceTypes = []func() client.Object{
	func() client.Object { return &corev1.Pod{} },
	func() client.Object { return &appsv1.Deployment{} },
	func() client.Object { return &appsv1.ReplicaSet{} },
	func() client.Object { return &appsv1.StatefulSet{} },
	func() client.Object { return &appsv1.DaemonSet{} },
	func() client.Object { return &batchv1.Job{} },
	func() client.Object { return &batchv1.CronJob{} },
}

func RegisterResourceMonitors(mgr manager.Manager) error {
	for _, r := range resourceTypes {
		if err := NewResourceMonitorController(mgr.GetClient(), r).SetupWithManager(mgr); err != nil {
			return err
		}
	}
	return nil
}
