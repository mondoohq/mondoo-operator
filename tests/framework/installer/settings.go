package installer

const MondooNamespace = "mondoo-operator"

type Settings struct {
	Namespace string
}

func NewDefaultSettings() Settings {
	return Settings{Namespace: MondooNamespace}
}
