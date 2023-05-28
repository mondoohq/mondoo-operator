package metrics

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testNamespace             = "mondoo-operator"
	testMondooCredsSecretName = "mondoo-client"
)

type MetricsReconcilerSuite struct {
	suite.Suite
	fakeClientBuilder *fake.ClientBuilder
}

var testLogger logr.Logger

func (s *MetricsReconcilerSuite) SetupSuite() {
	utilruntime.Must(clientgoscheme.AddToScheme(clientgoscheme.Scheme))
	utilruntime.Must(v1alpha2.AddToScheme(clientgoscheme.Scheme))

	// Setup logging
	var err error
	cfg := zap.NewDevelopmentConfig()

	cfg.InitialFields = map[string]interface{}{
		"controller": "metrics-test",
	}

	zapLog, err := cfg.Build()
	s.Require().NoError(err, "failed to set up logging for test cases")

	testLogger = zapr.NewLogger(zapLog)
}

func (s *MetricsReconcilerSuite) BeforeTest(suiteName, testName string) {
	s.fakeClientBuilder = fake.NewClientBuilder()
}

func (s *MetricsReconcilerSuite) TestMetricMondooAuditConfig() {
	mondooAuditConfig := testMondooAuditConfig()

	existingObjects := []runtime.Object{
		mondooAuditConfig,
	}
	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(existingObjects...).Build()
	mockCtrl := gomock.NewController(s.T())
	ctx := context.Background()
	mr := &MetricsReconciler{
		Client: fakeClient,
		ctx:    ctx,
		log:    testLogger,
	}
	mondooAuditConfigs := &v1alpha2.MondooAuditConfigList{}
	if err := mr.Client.List(mr.ctx, mondooAuditConfigs); err != nil {
		mr.log.Error(err, "error listing MondooAuditConfigs")
		return
	}
	mr.setMetricMondooAuditConfig(float64(len(mondooAuditConfigs.Items)))
	count := mr.getMetricMondooAuditConfig()
	s.Assert().Equal(float64(len(mondooAuditConfigs.Items)), count)
	mockCtrl.Finish()
}

func TestDeploymentHandlerSuite(t *testing.T) {
	suite.Run(t, new(MetricsReconcilerSuite))
}

func testMondooAuditConfig() *v1alpha2.MondooAuditConfig {
	return &v1alpha2.MondooAuditConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mondoo-config",
			Namespace: testNamespace,
		},
		Spec: v1alpha2.MondooAuditConfigSpec{
			MondooCredsSecretRef: corev1.LocalObjectReference{
				Name: testMondooCredsSecretName,
			},
		},
	}
}
