package installer

const MondooNamespace = "mondoo-operator"

type Settings struct {
	Namespace      string
	installRelease bool
}

func NewDefaultSettings() Settings {
	return Settings{Namespace: MondooNamespace, installRelease: false}
}

func NewReleaseSettings() Settings {
	return Settings{Namespace: MondooNamespace, installRelease: true}
}
