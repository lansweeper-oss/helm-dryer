package repocreds

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	secretTypeLabel = "argocd.argoproj.io/secret-type" //nolint:gosec // label key, not a credential
	namespaceFile   = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

// FetchFromCluster lists ArgoCD repository and repo-creds secrets from the Kubernetes API.
// When namespace is empty, it is auto-detected from the pod's service account mount.
func FetchFromCluster(ctx context.Context, namespace string) ([]RepoCred, []RepoCred, error) {
	if namespace == "" {
		ns, err := os.ReadFile(namespaceFile)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to detect namespace: %w", err)
		}

		namespace = strings.TrimSpace(string(ns))
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return fetchSecrets(ctx, clientset, namespace)
}

// fetchSecrets lists ArgoCD secrets and splits them into repos (exact-match credentials for a
// specific URL) and templates (prefix-match credentials that apply to any URL sharing the prefix).
func fetchSecrets(
	ctx context.Context, clientset kubernetes.Interface, namespace string,
) ([]RepoCred, []RepoCred, error) {
	list, err := clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: secretTypeLabel + " in (repository, repo-creds)",
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	var repos, templates []RepoCred

	for i := range list.Items {
		secret := &list.Items[i]
		cred := parseSecret(secret.Data)

		if cred.URL == "" {
			slog.Debug("Skipping ArgoCD secret with empty URL", "name", secret.Name)

			continue
		}

		secretType := secret.Labels[secretTypeLabel]

		slog.Debug("Found ArgoCD credential secret", "name", secret.Name, "type", secretType, "url", cred.URL)

		switch secretType {
		case "repository":
			repos = append(repos, cred)
		case "repo-creds":
			templates = append(templates, cred)
		}
	}

	slog.Debug("Loaded ArgoCD credentials", "repos", len(repos), "templates", len(templates))

	return repos, templates, nil
}

// parseSecret extracts credential fields from a Kubernetes secret's data map.
func parseSecret(data map[string][]byte) RepoCred {
	return RepoCred{
		URL:      string(data["url"]),
		Username: string(data["username"]),
		Password: string(data["password"]),
		TLSCert:  data["tlsClientCertData"],
		TLSKey:   data["tlsClientCertKey"],
	}
}
