// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mondoo.com/mondoo-operator/tests/framework/installer"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TestCluster struct {
	*installer.MondooInstaller
	managedBy string
	T         func() *testing.T
}

func StartTestCluster(ctx context.Context, settings installer.Settings, t func() *testing.T) *TestCluster {
	cfg := zap.NewDevelopmentConfig()
	logger, _ := cfg.Build()
	zap.ReplaceGlobals(logger)

	operatorInstaller := installer.NewMondooInstaller(settings, t)

	ns := &corev1.Namespace{}
	require.NoError(t(), operatorInstaller.K8sHelper.Clientset.Get(ctx, client.ObjectKey{Name: "kube-system"}, ns))

	cluster := &TestCluster{
		MondooInstaller: operatorInstaller,
		managedBy:       "mondoo-operator-" + string(ns.UID),
		T:               t,
	}

	if err := cluster.InstallOperator(); err != nil {
		zap.S().Errorf("Mondoo operator was not installed successfully: %v", err)
		if !t().Failed() {
			cluster.GatherAllMondooLogs(t().Name(), installer.MondooNamespace)
		}
		t().Fail()
		if err := cluster.UninstallOperator(); err != nil {
			zap.S().Errorf("Failed to uninstall Mondoo operator. %v", err)
		}
		t().FailNow()
	}

	return cluster
}

func (t *TestCluster) ManagedBy() string {
	return t.managedBy
}

func HandlePanics(r interface{}, uninstaller func(), t func() *testing.T) {
	if r != nil {
		zap.S().Infof("unexpected panic occurred during test %s, --> %v", t().Name(), r)
		t().Fail()
		uninstaller()
		t().FailNow()
	}
}
