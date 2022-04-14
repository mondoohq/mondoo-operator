/*
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"os"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	mondoov1alpha1 "go.mondoo.com/mondoo-operator/api/v1alpha1"
	webhookhandler "go.mondoo.com/mondoo-operator/pkg/webhooks/handler"
)

func init() {
	log.SetLogger(zap.New())
}

func main() {
	webhookLog := log.Log.WithName("webhook")

	// Setup a Manager
	webhookLog.Info("setting up manager")
	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{
		HealthProbeBindAddress: ":8081",
	})
	if err != nil {
		webhookLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	// Setup webhooks
	webhookLog.Info("setting up webhook server")
	hookServer := mgr.GetWebhookServer()

	// Determine whether we are in enforcing or permissive mode
	mode, exists := os.LookupEnv(mondoov1alpha1.WebhookModeEnvVar)
	if !exists {
		mode = string(mondoov1alpha1.Permissive)
	}

	webhookLog.Info("running with webhook configuration", "mode", mode)

	webhookLog.Info("registering webhooks to the webhook server")

	webhookValidator, err := webhookhandler.NewWebhookValidator(mgr.GetClient(), mode)
	if err != nil {
		webhookLog.Error(err, "failed to setup Core Webhook")
		os.Exit(1)
	}
	hookServer.Register("/validate-k8s-mondoo-com-core", &webhook.Admission{Handler: webhookValidator})

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		webhookLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		webhookLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	webhookLog.Info("starting manager")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		webhookLog.Error(err, "unable to run manager")
		os.Exit(1)
	}
}
