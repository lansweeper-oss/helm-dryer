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

	clientset := fake.NewSimpleClientset(repoSecret, credsSecret, secretMissingURL, unrelatedSecret)

	repos, templates, err := fetchSecrets(context.Background(), clientset, "argocd")
	require.NoError(t, err)

	store := NewStore(repos, templates)

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

	clientset := fake.NewSimpleClientset()

	repos, templates, err := fetchSecrets(context.Background(), clientset, "argocd")
	require.NoError(t, err)

	store := NewStore(repos, templates)
	assert.Nil(t, store.ForURL("https://anything.com"))
}

func TestNormalizeURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"lowercase scheme", "HTTPS://example.com/path", "https://example.com/path"},
		{"lowercase host", "https://EXAMPLE.COM/path", "https://example.com/path"},
		{"trim trailing slash", "https://example.com/", "https://example.com"},
		{"trim multiple trailing slashes", "https://example.com///", "https://example.com"},
		{"preserve path case", "https://example.com/MyChart", "https://example.com/MyChart"},
		{"OCI URL", "OCI://GHCR.IO/Org/Chart", "oci://ghcr.io/Org/Chart"},
		{"already normalized", "https://example.com", "https://example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, normalizeURL(tc.input))
		})
	}
}

func TestParseSecret(t *testing.T) {
	t.Parallel()

	t.Run("all fields present", func(t *testing.T) {
		t.Parallel()

		data := map[string][]byte{
			"url":               []byte("https://example.com"),
			"username":          []byte("user"),
			"password":          []byte("pass"),
			"tlsClientCertData": []byte("cert"),
			"tlsClientCertKey":  []byte("key"),
		}

		cred := parseSecret(data)
		assert.Equal(t, "https://example.com", cred.URL)
		assert.Equal(t, "user", cred.Username)
		assert.Equal(t, "pass", cred.Password)
		assert.Equal(t, []byte("cert"), cred.TLSCert)
		assert.Equal(t, []byte("key"), cred.TLSKey)
	})

	t.Run("missing fields return zero values", func(t *testing.T) {
		t.Parallel()

		cred := parseSecret(map[string][]byte{"url": []byte("https://example.com")})
		assert.Equal(t, "https://example.com", cred.URL)
		assert.Empty(t, cred.Username)
		assert.Empty(t, cred.Password)
		assert.Nil(t, cred.TLSCert)
		assert.Nil(t, cred.TLSKey)
	})

	t.Run("nil data returns empty cred", func(t *testing.T) {
		t.Parallel()

		cred := parseSecret(nil)
		assert.Empty(t, cred.URL)
	})
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

	clientset := fake.NewSimpleClientset(secret)

	repos, _, err := fetchSecrets(context.Background(), clientset, "argocd")
	require.NoError(t, err)
	require.Len(t, repos, 1)

	cred := &repos[0]
	require.NotNil(t, cred)
	assert.Equal(t, []byte("cert-pem-data"), cred.TLSCert)
	assert.Equal(t, []byte("key-pem-data"), cred.TLSKey)
	assert.Empty(t, cred.Username)
}
