// Package argo represents the ArgoCD Application CRD.
package argo

const (
	// Parameters is the name of the environment variable hosting the ArgoCD Application.
	Parameters = "ARGOCD_APP_PARAMETERS"
)

// App represents the ArgoCD Application CRD.
type App struct {
	Metadata argoAppMetadata `json:"metadata"`
	Spec     argoAppSpec     `json:"spec"`
}

// Just represent the name, we ignore the rest.
type argoAppMetadata struct {
	Name string `json:"name"`
}

type argoAppSpec struct {
	Project     string             `json:"project"`
	Source      argoAppSource      `json:"source"`
	Destination argoAppDestination `json:"destination"`
}

type argoAppDestination struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Server    string `json:"server"`
}

type argoAppSource struct {
	Path   string     `json:"path"`
	Plugin pluginSpec `json:"plugin"`
}

type pluginSpec struct {
	Parameters []Parameter `json:"parameters"`
}

// Parameter represents the input parameters in the ArgoCD Application CRD.
type Parameter struct {
	Array []string          `json:"array,omitempty"`
	Map   map[string]string `json:"map,omitempty"`
	Name  string            `json:"name"`
}
