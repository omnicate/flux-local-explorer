package resource

type ObjectMeta struct {
	Name      string
	Namespace string
}

func (o ObjectMeta) GetName() string {
	return o.Name
}

func (o ObjectMeta) GetNamespace() string {
	return o.Namespace
}

// CrossNamespaceObjectReference contains enough information to let you locate
// the typed referenced object at cluster level.
type CrossNamespaceObjectReference struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
}
