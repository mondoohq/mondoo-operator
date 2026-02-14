// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package integration

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

const (
	// How often to wake up and perform the integration CheckIn()
	interval = time.Minute * 10
)

var logger = log.Log.WithName("integration")

// Add creates a new Integrations controller adds it to the Manager.
func Add(mgr manager.Manager) error {
	cfg := zap.NewDevelopmentConfig()

	cfg.InitialFields = map[string]interface{}{
		"controller": "integration",
	}

	mc := &IntegrationReconciler{
		Client:              mgr.GetClient(),
		Interval:            interval,
		MondooClientBuilder: mondooclient.NewClient,
	}
	if err := mgr.Add(mc); err != nil {
		logger.Error(err, "failed to add integration controller to manager")
		return err
	}
	return nil
}

type IntegrationReconciler struct {
	Client client.Client

	// Interval is the length of time we sleep between runs
	Interval            time.Duration
	MondooClientBuilder func(mondooclient.MondooClientOptions) (mondooclient.MondooClient, error)
	ctx                 context.Context
}

// Start begins the integration status loop.
func (r *IntegrationReconciler) Start(ctx context.Context) error {
	logger.Info("started Mondoo console integration goroutine")

	r.ctx = ctx

	// Run forever, sleep at the end:
	wait.Until(r.integrationLoop, r.Interval, ctx.Done())

	return nil
}

func (r *IntegrationReconciler) integrationLoop() {
	logger.Info("Listing all MondooAuditConfigs")

	mondooAuditConfigs := &v1alpha2.MondooAuditConfigList{}
	if err := r.Client.List(r.ctx, mondooAuditConfigs); err != nil {
		logger.Error(err, "error listing MondooAuditConfigs")
		return
	}

	for _, mac := range mondooAuditConfigs.Items {
		if mac.Spec.ConsoleIntegration.Enable {
			if err := r.processMondooAuditConfig(mac); err != nil {
				logger.Error(err, "failed to process MondooAuditconfig", "mondooAuditConfig", fmt.Sprintf("%s/%s", mac.Namespace, mac.Name))
			}
		}
	}
}

func (r *IntegrationReconciler) processMondooAuditConfig(m v1alpha2.MondooAuditConfig) error {
	var err error
	defer func() {
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		_ = r.setIntegrationCondition(&m, err != nil, msg)
	}()

	secret, err := k8s.GetIntegrationSecretForAuditConfig(r.ctx, r.Client, m)
	if err != nil {
		return err
	}

	integrationMrn, err := k8s.GetIntegrationMrnFromSecret(*secret)
	if err != nil {
		return err
	}

	serviceAccount, err := k8s.GetServiceAccountFromSecret(*secret)
	if err != nil {
		return err
	}

	config := &v1alpha2.MondooOperatorConfig{}
	if err = r.Client.Get(r.ctx, types.NamespacedName{Name: v1alpha2.MondooOperatorConfigName}, config); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	}

	if err = mondoo.IntegrationCheckIn(r.ctx, integrationMrn, *serviceAccount, r.MondooClientBuilder, config.Spec.HttpProxy, config.Spec.HttpsProxy, config.Spec.NoProxy, logger); err != nil {
		logger.Error(err, "failed to CheckIn() for integration", "integrationMRN", string(integrationMrn))
		return err
	}

	return nil
}

func (r *IntegrationReconciler) setIntegrationCondition(config *v1alpha2.MondooAuditConfig, degradedStatus bool, customMessage string) error {
	originalConfig := config.DeepCopy()

	updateIntegrationCondition(config, degradedStatus, customMessage)

	if !reflect.DeepEqual(originalConfig.Status.Conditions, config.Status.Conditions) {
		logger.Info("status has changed, updating")
		if err := r.Client.Status().Update(r.ctx, config); err != nil {
			logger.Error(err, "failed to update status")
			return err
		}
	}

	return nil
}

func updateIntegrationCondition(config *v1alpha2.MondooAuditConfig, degradedStatus bool, customMessage string) {
	msg := "Mondoo integration is working"
	reason := "IntegrationAvailable"
	status := corev1.ConditionFalse
	updateCheck := mondoo.UpdateConditionIfReasonOrMessageChange
	if !config.Spec.ConsoleIntegration.Enable {
		msg = "Mondoo integration is disabled"
		reason = "IntegrationDisabled"
		status = corev1.ConditionFalse
	} else if degradedStatus {
		msg = "Mondoo integration not working"
		reason = "IntegrationUnavailable"
		status = corev1.ConditionTrue
	}

	// If user provided a custom message, use it
	if customMessage != "" {
		msg = customMessage
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(config.Status.Conditions, v1alpha2.MondooIntegrationDegraded, status, reason, msg, updateCheck, []string{}, "")
}
