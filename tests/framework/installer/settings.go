package installer

import "go.mondoo.com/mondoo-operator/tests/framework/utils"

const MondooNamespace = "mondoo-operator"

type Settings struct {
	Namespace      string
	token          string
	installRelease bool
	enableCnspec   bool
}

func (s Settings) EnableCnspec() Settings {
	s.enableCnspec = true
	return s
}

func (s Settings) GetEnableCnspec() bool {
	return s.enableCnspec
}

func (s Settings) SetToken(token string) Settings {
	s.token = token
	return s
}

func (s Settings) Token() string {
	// If the token is not set yet, read it from the local file.
	if s.token == "" {
		s.token = utils.ReadFile(MondooCredsFile)
	}
	return s.token
}

func NewDefaultSettings() Settings {
	return Settings{Namespace: MondooNamespace, installRelease: false}
}

func NewReleaseSettings() Settings {
	return Settings{Namespace: MondooNamespace, installRelease: true}
}
