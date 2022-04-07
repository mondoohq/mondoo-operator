package integration

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type E2ESuite struct {
	suite.Suite
	testCluster *TestCluster
}

func (s *E2ESuite) SetupSuite() {
	s.testCluster = StartTestCluster(s.T)
}

func (s *E2ESuite) TearDownSuite() {
	s.testCluster.UninstallOperator()
}

func (s *E2ESuite) TestExample() {
	s.Equal(1, 1)
}

func TestE2ESuite(t *testing.T) {
	s := new(E2ESuite)
	defer func(s *E2ESuite) {
		HandlePanics(recover(), s.TearDownSuite, s.T)
	}(s)
	suite.Run(t, s)
}
