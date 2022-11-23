package installer

const MondooNamespace = "mondoo-operator"

type Settings struct {
	Namespace      string
	installRelease bool
	enableCnspec   bool
}

func (s Settings) EnableCnspec() Settings {
	s.enableCnspec = true
	return s
}

func NewDefaultSettings() Settings {
	return Settings{Namespace: MondooNamespace, installRelease: false}
}

func NewReleaseSettings() Settings {
	return Settings{Namespace: MondooNamespace, installRelease: true}
}
