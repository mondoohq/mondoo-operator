package integration

type AuditConfigUpgradeSuite struct {
	AuditConfigBaseSuite
}

/*
func (s *AuditConfigUpgradeSuite) AfterTest(suiteName, testName string) {
	if s.testCluster != nil {
		s.testCluster.GatherAllMondooLogs(testName, installer.MondooNamespace)
	}
}

func (s *AuditConfigUpgradeSuite) TearDownSuite() {
	s.NoError(s.testCluster.UninstallOperator())
}

func (s *AuditConfigUpgradeSuite) TestUpgradePreviousReleaseToLatest() {
	auditConfig := utils.DefaultAuditConfigMinimal(s.testCluster.Settings.Namespace, true, true, true)
	s.testUpgradePreviousReleaseToLatest(auditConfig)
}

func TestAuditConfigUpgradeSuite(t *testing.T) {
	s := new(AuditConfigUpgradeSuite)
	s.installRelease = true
	defer func(s *AuditConfigUpgradeSuite) {
		HandlePanics(recover(), func() {
			if err := s.testCluster.UninstallOperator(); err != nil {
				zap.S().Errorf("Failed to uninstall Mondoo operator. %v", err)
			}
		}, s.T)
	}(s)
	suite.Run(t, s)
}
*/
