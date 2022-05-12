package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
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
		log:                 log,
		mondooClientBuilder: mondooclient.NewClient,
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
	log                 logr.Logger
	mondooClientBuilder func(mondooclient.ClientOptions) mondooclient.Client
	ctx                 context.Context
}

// Start begins the integration status loop.
func (r *IntegrationReconciler) Start(ctx context.Context) error {
	r.log.Info("started Mondoo console integration goroutine")

	r.ctx = ctx

	// Run forever, sleep at the end:
	wait.Until(r.integrationLoop, r.Interval, ctx.Done())

	return nil
}

func (r *IntegrationReconciler) integrationLoop() {

	r.log.Info("Listing all MondooAuditConfigs")

	mondooAuditConfigs := &v1alpha2.MondooAuditConfigList{}
	if err := r.Client.List(r.ctx, mondooAuditConfigs); err != nil {
		r.log.Error(err, "error listing MondooAuditConfigs")
		return
	}

	for _, mac := range mondooAuditConfigs.Items {
		if mac.Spec.ConsoleIntegration.Enable {
			if err := r.processMondooAuditConfig(mac); err != nil {
				r.log.Error(err, "failed to process MondooAuditconfig", "mondooAuditConfig", fmt.Sprintf("%s/%s", mac.Namespace, mac.Name))
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
		r.log.Error(err, "failed to read Mondoo creds from secret")
		return err
	}

	integrationMrn, ok := mondooCreds.Data[constants.MondooCredsSecretIntegrationMRNKey]
	if !ok {
		err := fmt.Errorf("cannot CheckIn() with 'integrationmrn' data missing from Mondoo creds secret")
		r.log.Error(err, "in order to perform a CheckIn(), the MondooAuditConfig.Spec.MondooCredsSecretRef must specify the integration MRN in the key 'integrationmrn'")
		return err
	}
	serviceAccount := &mondooclient.ServiceAccountCredentials{}
	if err := json.Unmarshal(mondooCreds.Data[constants.MondooCredsSecretServiceAccountKey], serviceAccount); err != nil {
		r.log.Error(err, "failed to unmarshal creds Secret")
		return err
	}
	token, err := r.generateTokenFromServiceAccount(serviceAccount)
	if err != nil {
		r.log.Error(err, "unable to generate token from service account")
		return err
	}
	mondooClient := r.mondooClientBuilder(mondooclient.ClientOptions{
		ApiEndpoint: serviceAccount.ApiEndpoint,
		Token:       token,
	})

	// Do the actual check-in
	if _, err := mondooClient.IntegrationCheckIn(r.ctx, &mondooclient.IntegrationCheckInInput{
		Mrn: string(integrationMrn),
	}); err != nil {
		r.log.Error(err, "failed to CheckIn() to Mondoo API")
		return err
	}
	return nil
}

func (r *IntegrationReconciler) generateTokenFromServiceAccount(serviceAccount *mondooclient.ServiceAccountCredentials) (string, error) {
	block, _ := pem.Decode([]byte(serviceAccount.PrivateKey))
	if block == nil {
		err := fmt.Errorf("found no PEM block in private key")
		r.log.Error(err, "failed to decode service account's private key")
		return "", err
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		r.log.Error(err, "failed to parse private key data")
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
		r.log.Error(err, "failed to generate token")
		return "", err
	}

	return tokenString, nil
}
