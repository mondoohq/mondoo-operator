package integration

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

const (
	// How often to wake up and perform the integration CheckIn()
	interval = time.Minute * 10
)

// Add creates a new Integrations controller adds it to the Manager.
func Add(mgr manager.Manager) error {
	var log logr.Logger

	cfg := zap.NewDevelopmentConfig()

	cfg.InitialFields = map[string]interface{}{
		"controller": "integration",
	}

	zapLog, err := cfg.Build()
	if err != nil {
		return fmt.Errorf("failed to set up logging for integration controller: %s", err)
	}
	log = zapr.NewLogger(zapLog)

	mc := &IntegrationReconciler{
		Client:              mgr.GetClient(),
		Interval:            interval,
		Log:                 log,
		MondooClientBuilder: mondooclient.NewClient,
	}
	if err := mgr.Add(mc); err != nil {
		log.Error(err, "failed to add integration controller to manager")
		return err
	}
	return nil
}

type IntegrationReconciler struct {
	Client client.Client

	// Interval is the length of time we sleep between runs
	Interval            time.Duration
	Log                 logr.Logger
	MondooClientBuilder func(mondooclient.ClientOptions) mondooclient.Client
	ctx                 context.Context
}

// Start begins the integration status loop.
func (r *IntegrationReconciler) Start(ctx context.Context) error {
	r.Log.Info("started Mondoo console integration goroutine")

	r.ctx = ctx

	// Run forever, sleep at the end:
	wait.Until(r.integrationLoop, r.Interval, ctx.Done())

	return nil
}

func (r *IntegrationReconciler) integrationLoop() {
	r.Log.Info("Listing all MondooAuditConfigs")

	mondooAuditConfigs := &v1alpha2.MondooAuditConfigList{}
	if err := r.Client.List(r.ctx, mondooAuditConfigs); err != nil {
		r.Log.Error(err, "error listing MondooAuditConfigs")
		return
	}

	for _, mac := range mondooAuditConfigs.Items {
		if mac.Spec.ConsoleIntegration.Enable {
			if err := r.processMondooAuditConfig(mac); err != nil {
				r.Log.Error(err, "failed to process MondooAuditconfig", "mondooAuditConfig", fmt.Sprintf("%s/%s", mac.Namespace, mac.Name))
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

	integrationMrn, err := k8s.GetIntegrationMrnFromSecret(*secret, m)
	if err != nil {
		return err
	}

	serviceAccount, err := k8s.GetServiceAccountFromSecret(*secret)
	if err != nil {
		return err
	}

	if err = r.IntegrationCheckIn(integrationMrn, *serviceAccount); err != nil {
		r.Log.Error(err, "failed to CheckIn() for integration", "integrationMRN", string(integrationMrn))
		return err
	}

	return nil
}

func (r *IntegrationReconciler) IntegrationCheckIn(integrationMrn string, sa mondooclient.ServiceAccountCredentials) error {
	token, err := mondoo.GenerateTokenFromServiceAccount(sa, r.Log)
	if err != nil {
		msg := "unable to generate token from service account"
		r.Log.Error(err, msg)
		return fmt.Errorf("%s: %s", msg, err)
	}
	mondooClient := r.MondooClientBuilder(mondooclient.ClientOptions{
		ApiEndpoint: sa.ApiEndpoint,
		Token:       token,
	})

	// Do the actual check-in
	if _, err := mondooClient.IntegrationCheckIn(r.ctx, &mondooclient.IntegrationCheckInInput{
		Mrn: integrationMrn,
	}); err != nil {
		msg := "failed to CheckIn() to Mondoo API"
		r.Log.Error(err, msg)
		return fmt.Errorf("%s: %s", msg, err)
	}

	return nil
}

func (r *IntegrationReconciler) setIntegrationCondition(config *v1alpha2.MondooAuditConfig, degradedStatus bool, customMessage string) error {
	originalConfig := config.DeepCopy()

	updateIntegrationCondition(config, degradedStatus, customMessage)

	if !reflect.DeepEqual(originalConfig.Status.Conditions, config.Status.Conditions) {
		r.Log.Info("status has changed, updating")
		if err := r.Client.Status().Update(r.ctx, config); err != nil {
			r.Log.Error(err, "failed to update status")
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
	if degradedStatus {
		msg = "Mondoo integration not working"
		reason = "IntegrationUnvailable"
		status = corev1.ConditionTrue
	}

	// If user provided a custom message, use it
	if customMessage != "" {
		msg = customMessage
	}

	config.Status.Conditions = mondoo.SetMondooAuditCondition(config.Status.Conditions, v1alpha2.MondooIntegrationDegraded, status, reason, msg, updateCheck)
}
