package integration

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/tests/framework/installer"
)

type MondooInstallationSuite struct {
	suite.Suite
	testCluster *TestCluster
}

func (s *MondooInstallationSuite) SetupSuite() {

}

func (s *MondooInstallationSuite) TearDownSuite() {
	s.testCluster.UninstallOperator()
}

func (s *MondooInstallationSuite) AfterTest(suiteName, testName string) {
	if !s.T().Failed() {
		s.testCluster.GatherAllMondooLogs(testName, installer.MondooNamespace)
	}
	s.testCluster.UninstallOperator()
}

func (s *MondooInstallationSuite) TestKustomizeInstallation() {
	s.testCluster = StartTestCluster(installer.NewDefaultSettings(), s.T)
}

func (s *MondooInstallationSuite) TestKustomizeInstallation_NonDefaultNamespace() {
	settings := installer.NewDefaultSettings()
	settings.Namespace = "some-namespace"
	s.testCluster = StartTestCluster(settings, s.T)
}

func TestMondooInstallationSuite(t *testing.T) {
	s := new(MondooInstallationSuite)
	defer func(s *MondooInstallationSuite) {
		HandlePanics(recover(), s.TearDownSuite, s.T)
	}(s)
	suite.Run(t, s)
}
