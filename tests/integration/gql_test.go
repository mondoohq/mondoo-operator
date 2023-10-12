package integration

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/suite"
	"go.mondoo.com/mondoo-operator/tests/framework/nexus"
	"go.mondoo.com/mondoo-operator/tests/framework/utils"

	mondoogql "go.mondoo.com/mondoo-go"
)

type GqlSuite struct {
	suite.Suite
	ctx         context.Context
	spaceClient *nexus.Space
	nexusClient *nexus.Client
}

func (s *GqlSuite) SetupSuite() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	s.ctx = context.Background()

	sa, err := utils.GetServiceAccount()
	s.Require().NoError(err, "Service account not set")
	nexusClient, err := nexus.NewClient(sa)
	s.Require().NoError(err, "Failed to create Nexus client")

	s.nexusClient = nexusClient
}

func (s *GqlSuite) TestCreateSpace() {
	// s.spaceClient = s.nexusClient.GetSpace()
	// err := s.spaceClient.Delete(context.Background())
	// 29GPDsplmmH9Nt5XqhhHjiSqRpk
	var q struct {
		CiCdProjects struct {
			Projects struct {
				Projects struct {
					Edges []struct {
						Node struct {
							Mrn    string
							Labels []struct {
								Key   string
								Value string
							}
						}
					}
				} `graphql:"projects(first: $first)"`
			} `graphql:"... on CicdProjects"`
		} `graphql:"cicdProjects(input: $input)"`
	}

	err := s.nexusClient.Client.Query(s.ctx, &q, map[string]interface{}{"input": mondoogql.CicdProjectsInput{SpaceMrn: "//captain.api.mondoo.app/spaces/dreamy-wilson-259171"}, "first": mondoogql.Int(100)})
	s.NoError(err)
	// assets, err := assets.ListAssetsWithScores(
	// 	context.Background(),
	// 	"//captain.api.mondoo.app/spaces/dreamy-wilson-259171",
	// 	"",
	// 	s.nexusClient.Client)
	// s.NotEmpty(assets)
	// s.NoError(err)
}

func TestGqlSuite(t *testing.T) {
	s := new(GqlSuite)
	suite.Run(t, s)
}
