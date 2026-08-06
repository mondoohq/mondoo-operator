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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/client/common"
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
		if mac.ConsoleIntegrationActive() {
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

	existingHash := m.Status.RemoteConfigHash

	result, err := mondoo.IntegrationCheckIn(r.ctx, integrationMrn, existingHash, *serviceAccount, r.MondooClientBuilder, config.Spec.HttpProxy, config.Spec.HttpsProxy, config.Spec.NoProxy, logger)
	if err != nil {
		// A 404 means the integration was deleted in the Mondoo console. The operator does
		// not recreate it on its own: re-provisioning only happens when the creds Secret is
		// deleted, so an integration never reappears unexpectedly.
		if common.IsNotFound(err) {
			err = fmt.Errorf(
				"integration %s no longer exists in the Mondoo console; check-ins, status reporting and pause/unpause are disabled while scanning continues. "+
					"Delete the Secret %s/%s to re-provision, or disable .spec.consoleIntegration", integrationMrn, secret.Namespace, secret.Name)
		}
		logger.Error(err, "failed to CheckIn() for integration", "integrationMRN", string(integrationMrn))
		if m.Spec.RemoteManaged {
			setRemoteConfigDegradedCondition(&m, true, "FetchFailed", err.Error())
			if updateErr := r.Client.Status().Update(r.ctx, &m); updateErr != nil {
				logger.Error(updateErr, "failed to update RemoteConfigDegraded condition")
			}
		}
		return err
	}

	statusChanged := false

	if result != nil && result.ConfigFetched {
		if result.Paused != m.Status.ScanningPaused {
			m.Status.ScanningPaused = result.Paused
			statusChanged = true
		}

		if result.RawConfig != "" && result.ConfigurationHash != existingHash {
			m.Status.RemoteConfig = result.RawConfig
			m.Status.RemoteConfigHash = result.ConfigurationHash
			now := metav1.Now()
			m.Status.LastRemoteConfigTime = &now
			statusChanged = true
			logger.Info("persisted remote config to status", "newHash", result.ConfigurationHash)
		}

		if m.Spec.RemoteManaged {
			origConditions := m.DeepCopy().Status.Conditions
			setRemoteConfigDegradedCondition(&m, false, "", "")
			if !reflect.DeepEqual(origConditions, m.Status.Conditions) {
				statusChanged = true
			}
		}
	}

	if statusChanged {
		if updateErr := r.Client.Status().Update(r.ctx, &m); updateErr != nil {
			logger.Error(updateErr, "failed to update status")
			return updateErr
		}
	}

	return r.setScanningPausedCondition(&m)
}

func (r *IntegrationReconciler) setScanningPausedCondition(config *v1alpha2.MondooAuditConfig) error {
	originalConfig := config.DeepCopy()

	if config.Status.ScanningPaused {
		config.Status.Conditions = mondoo.SetMondooAuditCondition(
			config.Status.Conditions, v1alpha2.ScanningPausedCondition, corev1.ConditionTrue,
			"ScanningPaused", "Scanning has been paused from the Mondoo console",
			mondoo.UpdateConditionIfReasonOrMessageChange, nil, "",
		)
	} else {
		config.Status.Conditions = mondoo.SetMondooAuditCondition(
			config.Status.Conditions, v1alpha2.ScanningPausedCondition, corev1.ConditionFalse,
			"ScanningActive", "Scanning is active",
			mondoo.UpdateConditionIfReasonOrMessageChange, nil, "",
		)
	}

	if !reflect.DeepEqual(originalConfig.Status.Conditions, config.Status.Conditions) {
		return r.Client.Status().Update(r.ctx, config)
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
	if !config.ConsoleIntegrationActive() {
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

func setRemoteConfigDegradedCondition(config *v1alpha2.MondooAuditConfig, degraded bool, reason, message string) {
	status := corev1.ConditionFalse
	if degraded {
		status = corev1.ConditionTrue
	} else {
		reason = "ConfigFetched"
		message = "Remote configuration successfully fetched"
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(
		config.Status.Conditions, v1alpha2.RemoteConfigDegradedCondition, status,
		reason, message, mondoo.UpdateConditionIfReasonOrMessageChange, nil, "",
	)
}
