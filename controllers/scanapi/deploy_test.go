package scanapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mondoov1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
)

type DeploySuite struct {
	suite.Suite
	ctx    context.Context
	scheme *runtime.Scheme
	image  string
}

func (s *DeploySuite) SetupSuite() {
	s.ctx = context.Background()
	s.scheme = clientgoscheme.Scheme
	s.Require().NoError(mondoov1alpha2.AddToScheme(s.scheme))
	s.image = mondoo.MondooOperatorImage + ":" + mondoo.MondooOperatorTag
}

func (s *DeploySuite) TestDeploy_Create() {
	ns := "test-ns"
	auditConfig := utils.DefaultAuditConfig(ns, false, false, true)

	kubeClient := fake.NewClientBuilder().WithScheme(s.scheme).Build()

	s.NoError(Deploy(s.ctx, kubeClient, ns, s.image, auditConfig))

	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: auditConfig.Namespace,
			Name:      SecretName(auditConfig.Name),
		},
	}
	s.NoError(kubeClient.Get(s.ctx, client.ObjectKeyFromObject(tokenSecret), tokenSecret), "Error checking for token secret")
	// This really should be checking tokenSecret.Data, but the fake kubeClient just takes and stores the objects given to it
	// and our code populates the Secret through Secret.StringData["token"]
	s.Contains(tokenSecret.StringData, "token")

	ds := &appsv1.DeploymentList{}
	s.NoError(kubeClient.List(s.ctx, ds))
	s.Equal(1, len(ds.Items))

	d := ScanApiDeployment(ns, s.image, auditConfig)
	d.ResourceVersion = "1" // Needed because the fake client sets it.
	s.NoError(ctrl.SetControllerReference(&auditConfig, d, s.scheme))
	s.Equal(*d, ds.Items[0])

	ss := &corev1.ServiceList{}
	s.NoError(kubeClient.List(s.ctx, ss))
	s.Equal(1, len(ss.Items))

	service := ScanApiService(ns, auditConfig)
	service.ResourceVersion = "1" // Needed because the fake client sets it.
	s.NoError(ctrl.SetControllerReference(&auditConfig, service, s.scheme))
	s.Equal(*service, ss.Items[0])
}

func (s *DeploySuite) TestDeploy_Update() {
	ns := "test-ns"
	auditConfig := utils.DefaultAuditConfig(ns, false, false, true)
	d := ScanApiDeployment(ns, s.image, auditConfig)
	d.Spec.Replicas = pointer.Int32(3)

	service := ScanApiService(ns, auditConfig)
	service.Spec.Ports[0].Port = 1234

	client := fake.NewClientBuilder().WithObjects(d, service).WithScheme(s.scheme).Build()

	s.NoError(Deploy(s.ctx, client, ns, s.image, auditConfig))

	ds := &appsv1.DeploymentList{}
	s.NoError(client.List(s.ctx, ds))
	s.Equal(1, len(ds.Items))

	d = ScanApiDeployment(ns, s.image, auditConfig)
	s.NoError(ctrl.SetControllerReference(&auditConfig, d, s.scheme))
	d.ResourceVersion = "1000" // Needed because the fake client sets it.

	s.True(k8s.AreDeploymentsEqual(*d, ds.Items[0]))

	ss := &corev1.ServiceList{}
	s.NoError(client.List(s.ctx, ss))
	s.Equal(1, len(ss.Items))

	service = ScanApiService(ns, auditConfig)
	s.NoError(ctrl.SetControllerReference(&auditConfig, service, s.scheme))
	service.ResourceVersion = "1000" // Needed because the fake client sets it.

	s.True(k8s.AreServicesEqual(*service, ss.Items[0]))
}

func (s *DeploySuite) TestCleanup() {
	ns := "test-ns"
	auditConfig := utils.DefaultAuditConfig(ns, false, false, true)
	d := ScanApiDeployment(ns, s.image, auditConfig)
	service := ScanApiService(ns, auditConfig)
	client := fake.NewClientBuilder().WithObjects(d, service).Build()

	s.NoError(Cleanup(s.ctx, client, ns, auditConfig))

	ds := &appsv1.DeploymentList{}
	s.NoError(client.List(s.ctx, ds))
	s.Equal(0, len(ds.Items))

	sec := &corev1.SecretList{}
	s.NoError(client.List(s.ctx, sec))
	s.Equal(0, len(sec.Items))

	ss := &corev1.ServiceList{}
	s.NoError(client.List(s.ctx, ss))
	s.Equal(0, len(ss.Items))
}

func (s *DeploySuite) TestCleanup_AlreadyClean() {
	ns := "test-ns"
	auditConfig := utils.DefaultAuditConfig(ns, false, false, true)
	client := fake.NewClientBuilder().Build()

	s.NoError(Cleanup(s.ctx, client, ns, auditConfig))

	ds := &appsv1.DeploymentList{}
	s.NoError(client.List(s.ctx, ds))
	s.Equal(0, len(ds.Items))

	ss := &corev1.ServiceList{}
	s.NoError(client.List(s.ctx, ss))
	s.Equal(0, len(ss.Items))
}

func TestDeploySuite(t *testing.T) {
	suite.Run(t, new(DeploySuite))
}
