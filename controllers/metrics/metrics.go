// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package metrics

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"k8s.io/apimachinery/pkg/util/wait"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
)

var metricsMondooAuditConfigTotal = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "mondoo_audit_configs",
		Help: "Number of Mondoo audit configs",
	},
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(metricsMondooAuditConfigTotal)
}

// Add creates a new metrics reconciler and adds it to the Manager.
func Add(mgr manager.Manager) error {
	logger := log.Log.WithName("metrics")
	mr := &MetricsReconciler{
		Client:   mgr.GetClient(),
		Interval: 10 * time.Second,
		log:      logger,
	}
	err := mgr.Add(mr)
	if err != nil {
		logger.Error(err, "failed to add metrics controller to manager")
		return err
	}
	return nil
}

type MetricsReconciler struct {
	Client client.Client

	// Interval is the length of time we sleep between metrics calculations.
	Interval time.Duration

	log logr.Logger
	ctx context.Context
}

// Start begins the metrics loop.
func (mr *MetricsReconciler) Start(ctx context.Context) error {
	mr.log.Info("started metrics controller goroutine")

	mr.ctx = ctx
	// Run forever, sleep at the end:
	wait.Until(mr.metricsLoop, mr.Interval, ctx.Done())

	return nil
}

func (mr *MetricsReconciler) metricsLoop() {
	mr.log.Info("Updating metrics")
	mondooAuditConfigs := &v1alpha2.MondooAuditConfigList{}
	if err := mr.Client.List(mr.ctx, mondooAuditConfigs); err != nil {
		mr.log.Error(err, "error listing MondooAuditConfigs")
		return
	}
	mr.setMetricMondooAuditConfig(float64(len(mondooAuditConfigs.Items)))
}

func (mr *MetricsReconciler) setMetricMondooAuditConfig(num float64) {
	metricsMondooAuditConfigTotal.Set(num)
}

func (mr *MetricsReconciler) getMetricMondooAuditConfig() float64 {
	m := &dto.Metric{}
	if err := metricsMondooAuditConfigTotal.Write(m); err != nil {
		panic("failed to get metric value")
	}
	return m.Gauge.GetValue()
}
