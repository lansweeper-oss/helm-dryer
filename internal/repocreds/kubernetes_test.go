package repocreds //nolint:testpackage // tests unexported fetchSecrets

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestFetchSecrets(t *testing.T) {
	t.Parallel()

	repoSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "repo-helm-charts",
			Namespace: "argocd",
			Labels:    map[string]string{secretTypeLabel: "repository"},
		},
		Data: map[string][]byte{
			"url":      []byte("https://charts.example.com"),
			"username": []byte("repo-user"),
			"password": []byte("repo-pass"),
		},
	}

	credsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "creds-ghcr",
			Namespace: "argocd",
			Labels:    map[string]string{secretTypeLabel: "repo-creds"},
		},
		Data: map[string][]byte{
			"url":      []byte("oci://ghcr.io/org"),
			"username": []byte("oci-user"),
			"password": []byte("oci-pass"),
		},
	}

	secretMissingURL := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bad-secret",
			Namespace: "argocd",
			Labels:    map[string]string{secretTypeLabel: "repo-creds"},
		},
		Data: map[string][]byte{
			"username": []byte("no-url-user"),
		},
	}

	unrelatedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-secret",
			Namespace: "argocd",
		},
		Data: map[string][]byte{
			"url":      []byte("https://should-not-appear.com"),
			"username": []byte("nope"),
		},
	}

	//nolint:staticcheck // NewClientset requires applyconfig generation
	clientset := fake.NewSimpleClientset(repoSecret, credsSecret, secretMissingURL, unrelatedSecret)

	store, err := fetchSecrets(context.Background(), clientset, "argocd")
	require.NoError(t, err)
	require.NotNil(t, store)

	// Repository secret: exact match
	cred := store.ForURL("https://charts.example.com")
	require.NotNil(t, cred)
	assert.Equal(t, "repo-user", cred.Username)
	assert.Equal(t, "repo-pass", cred.Password)

	// Repo-creds template: prefix match
	cred = store.ForURL("oci://ghcr.io/org/my-chart")
	require.NotNil(t, cred)
	assert.Equal(t, "oci-user", cred.Username)

	// Secret without URL field was skipped
	// Unrelated secret (no label) was excluded by selector
	cred = store.ForURL("https://should-not-appear.com")
	assert.Nil(t, cred)
}

func TestFetchSecrets_EmptyNamespace(t *testing.T) {
	t.Parallel()

	//nolint:staticcheck // NewClientset requires applyconfig generation
	clientset := fake.NewSimpleClientset()

	store, err := fetchSecrets(context.Background(), clientset, "argocd")
	require.NoError(t, err)
	require.NotNil(t, store)
	assert.Nil(t, store.ForURL("https://anything.com"))
}

func TestFetchSecrets_TLSCertData(t *testing.T) {
	t.Parallel()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-repo",
			Namespace: "argocd",
			Labels:    map[string]string{secretTypeLabel: "repository"},
		},
		Data: map[string][]byte{
			"url":               []byte("https://secure.example.com"),
			"tlsClientCertData": []byte("cert-pem-data"),
			"tlsClientCertKey":  []byte("key-pem-data"),
		},
	}

	//nolint:staticcheck // NewClientset requires applyconfig generation
	clientset := fake.NewSimpleClientset(secret)

	store, err := fetchSecrets(context.Background(), clientset, "argocd")
	require.NoError(t, err)

	cred := store.ForURL("https://secure.example.com")
	require.NotNil(t, cred)
	assert.Equal(t, []byte("cert-pem-data"), cred.TLSCert)
	assert.Equal(t, []byte("key-pem-data"), cred.TLSKey)
	assert.Empty(t, cred.Username)
}
