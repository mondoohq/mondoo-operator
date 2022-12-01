package plugins

import (
	"fmt"
	"net/http"

	"go.mondoo.com/ranger-rpc"
)

func NewBearerAuthPlugin(token string) ranger.ClientPlugin {
	return &bearerAuthPlugin{token: token}
}

type bearerAuthPlugin struct {
	token string
}

// GetHeader implements ranger.ClientPlugin
func (p *bearerAuthPlugin) GetHeader(content []byte) http.Header {
	header := make(http.Header)

	header.Set("Authorization", fmt.Sprintf("Bearer %s", p.token))
	return header
}

// GetName implements ranger.ClientPlugin
func (*bearerAuthPlugin) GetName() string {
	return "BearerAuth"
}
