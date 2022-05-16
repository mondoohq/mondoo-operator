package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"

	jwt "github.com/golang-jwt/jwt/v4"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/constants"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
)

const (
	// How often to wake up and perform the integration CheckIn()
	interval = time.Minute * 10

	// must be set to "mondoo/ams" when making Mondoo API calls
	tokenIssuer = "mondoo/ams"
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

func (r *IntegrationReconciler) processMondooAuditConfig(mondoo v1alpha2.MondooAuditConfig) error {

	// Need to fetch the Secret with the creds, and use that to sign our own JWT
	mondooCreds := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mondoo.Spec.MondooCredsSecretRef.Name,
			Namespace: mondoo.Namespace,
		},
	}
	if err := r.Client.Get(r.ctx, client.ObjectKeyFromObject(mondooCreds), mondooCreds); err != nil {
		msg := "failed to read Mondoo creds from secret"
		r.Log.Error(err, msg)
		_ = r.setIntegrationCondition(&mondoo, true, fmt.Sprintf("%s: %s", msg, err))
		return err
	}

	integrationMrn, ok := mondooCreds.Data[constants.MondooCredsSecretIntegrationMRNKey]
	if !ok {
		msg := "cannot CheckIn() with 'integrationmrn' data missing from Mondoo creds secret"
		err := fmt.Errorf(msg)
		r.Log.Error(err, "in order to perform a CheckIn(), the MondooAuditConfig.Spec.MondooCredsSecretRef must specify the integration MRN in the key 'integrationmrn'")
		_ = r.setIntegrationCondition(&mondoo, true, msg)
		return err
	}

	serviceAccount := mondooCreds.Data[constants.MondooCredsSecretServiceAccountKey]

	if err := r.IntegrationCheckIn(integrationMrn, serviceAccount); err != nil {
		r.Log.Error(err, "failed to CheckIn() for integration", "integrationMRN", string(integrationMrn))
		_ = r.setIntegrationCondition(&mondoo, true, err.Error())
		return err
	}

	_ = r.setIntegrationCondition(&mondoo, false, "")

	return nil
}

func (r *IntegrationReconciler) IntegrationCheckIn(integrationMrn, serviceAccountBytes []byte) error {
	serviceAccount := &mondooclient.ServiceAccountCredentials{}
	if err := json.Unmarshal(serviceAccountBytes, serviceAccount); err != nil {
		msg := "failed to unmarshal creds Secret"
		r.Log.Error(err, msg)
		return fmt.Errorf("%s: %s", msg, err)
	}

	token, err := r.generateTokenFromServiceAccount(serviceAccount)
	if err != nil {
		msg := "unable to generate token from service account"
		r.Log.Error(err, msg)
		return fmt.Errorf("%s: %s", msg, err)
	}
	mondooClient := r.MondooClientBuilder(mondooclient.ClientOptions{
		ApiEndpoint: serviceAccount.ApiEndpoint,
		Token:       token,
	})

	// Do the actual check-in
	if _, err := mondooClient.IntegrationCheckIn(r.ctx, &mondooclient.IntegrationCheckInInput{
		Mrn: string(integrationMrn),
	}); err != nil {
		msg := "failed to CheckIn() to Mondoo API"
		r.Log.Error(err, msg)
		return fmt.Errorf("%s: %s", msg, err)
	}

	return nil
}

func (r *IntegrationReconciler) generateTokenFromServiceAccount(serviceAccount *mondooclient.ServiceAccountCredentials) (string, error) {
	block, _ := pem.Decode([]byte(serviceAccount.PrivateKey))
	if block == nil {
		err := fmt.Errorf("found no PEM block in private key")
		r.Log.Error(err, "failed to decode service account's private key")
		return "", err
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		r.Log.Error(err, "failed to parse private key data")
		return "", err
	}
	switch pk := key.(type) {
	case *ecdsa.PrivateKey:
		return r.createSignedToken(pk, serviceAccount)
	default:
		return "", fmt.Errorf("AuthKey must be of type ecdsa.PrivateKey")
	}
}

func (r *IntegrationReconciler) createSignedToken(pk *ecdsa.PrivateKey, sa *mondooclient.ServiceAccountCredentials) (string, error) {
	issuedAt := time.Now().Unix()

	token := jwt.NewWithClaims(jwt.SigningMethodES384, jwt.MapClaims{
		"sub": sa.Mrn,
		"iss": tokenIssuer,
		"iat": issuedAt,
		"exp": issuedAt + 60, // expire in 1 minute
		"nbf": issuedAt,
	})

	token.Header["kid"] = sa.Mrn

	tokenString, err := token.SignedString(pk)
	if err != nil {
		r.Log.Error(err, "failed to generate token")
		return "", err
	}

	return tokenString, nil
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
