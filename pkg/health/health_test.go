package health

import (
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/version"
)

const (
	mondooNamespace = "mondoo-operator"
	otherNamespace  = "some-namespace"
)

var (
	testLogger logr.Logger
)

type IntegrationCheckInSuite struct {
	suite.Suite
}

func TestMondooIntegrationCheckInSuite(t *testing.T) {
	suite.Run(t, new(IntegrationCheckInSuite))
}

func (s *IntegrationCheckInSuite) SetupSuite() {
	utilruntime.Must(v1alpha2.AddToScheme(clientgoscheme.Scheme))

	// Setup logging
	var err error
	cfg := zap.NewDevelopmentConfig()

	cfg.InitialFields = map[string]interface{}{
		"controller": "integration-test",
	}

	zapLog, err := cfg.Build()
	s.Require().NoError(err, "failed to set up logging for test cases")

	testLogger = zapr.NewLogger(zapLog)

}

func (s *IntegrationCheckInSuite) TestReconciledMondooAuditConfig() {
	// Arrange
	mondooAuditConfig := createReconciledMondooAuditConfig(mondooNamespace)

	existingObjects := []runtime.Object{
		mondooAuditConfig,
	}

	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(existingObjects...).Build()

	hc := &HealthChecks{
		Client: fakeClient,
		Log:    testLogger,
	}

	// Act
	err := hc.AreAllMondooAuditConfigsReconciled(&http.Request{})

	// Assert
	s.NoError(err, "should not error while processing reconciled MondooAuditConfig")
}

func (s *IntegrationCheckInSuite) TestReconciledMondooAuditConfigs() {
	// Arrange
	mondooAuditConfig := createReconciledMondooAuditConfig(mondooNamespace)
	mondooAuditConfigOtherNamespace := createReconciledMondooAuditConfig(otherNamespace)

	existingObjects := []runtime.Object{
		mondooAuditConfig,
		mondooAuditConfigOtherNamespace,
	}

	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(existingObjects...).Build()

	hc := &HealthChecks{
		Client: fakeClient,
		Log:    testLogger,
	}

	// Act
	err := hc.AreAllMondooAuditConfigsReconciled(&http.Request{})

	// Assert
	s.NoError(err, "should not error while processing reconciled MondooAuditConfigs across namespaces")
}

func (s *IntegrationCheckInSuite) TestUnfinishedMondooAuditConfig() {
	// Arrange
	mondooAuditConfig := createReconciledMondooAuditConfig(mondooNamespace)
	unreconciledMACOtherNamespace := createMondooAuditConfig(otherNamespace)

	existingObjects := []runtime.Object{
		mondooAuditConfig,
		unreconciledMACOtherNamespace,
	}

	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(existingObjects...).Build()

	hc := &HealthChecks{
		Client: fakeClient,
		Log:    testLogger,
	}

	// Act
	err := hc.AreAllMondooAuditConfigsReconciled(&http.Request{})

	// Assert
	s.Error(err, "should error while processing not yet reconciled MondooAuditConfig")
}

func createMondooAuditConfig(namespace string) *v1alpha2.MondooAuditConfig {
	return &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mondoo-config",
			Namespace: namespace,
		},
	}
}

func createReconciledMondooAuditConfig(namespace string) *v1alpha2.MondooAuditConfig {
	mondooAuditConfig := createMondooAuditConfig(namespace)
	mondooAuditConfig.Status.ReconciledByOperatorVersion = version.Version
	return mondooAuditConfig
}
