package client //nolint:testpackage // tests unexported helpers

import (
	"os"
	"testing"

	"github.com/lansweeper-oss/helm-dryer/internal/repocreds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractOCIHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		repoURL  string
		expected string
	}{
		{"standard OCI URL", "oci://ghcr.io/org/chart", "ghcr.io"},
		{"OCI URL no path", "oci://ghcr.io", "ghcr.io"},
		{"OCI URL with port", "oci://registry.example.com:5000/charts", "registry.example.com:5000"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, extractOCIHost(tc.repoURL))
		})
	}
}

func TestCredForURL(t *testing.T) {
	t.Parallel()

	store := repocreds.NewStore(nil, []repocreds.RepoCred{
		{URL: "https://charts.example.com", Username: "user1", Password: "pass1"},
	})

	t.Run("matching URL returns credentials", func(t *testing.T) {
		t.Parallel()

		c := &Client{CredsStore: store}
		rc := c.credForURL("https://charts.example.com/mychart")

		assert.Equal(t, "user1", rc.Username)
		assert.Equal(t, "pass1", rc.Password)
	})

	t.Run("no match returns empty cred", func(t *testing.T) {
		t.Parallel()

		c := &Client{CredsStore: store}
		rc := c.credForURL("https://unknown.example.com")

		assert.Empty(t, rc.Username)
		assert.Empty(t, rc.Password)
	})

	t.Run("nil store returns empty cred", func(t *testing.T) {
		t.Parallel()

		c := &Client{}
		rc := c.credForURL("https://charts.example.com")

		assert.Empty(t, rc.Username)
	})

	t.Run("TLS certs written to temp files and cleaned up", func(t *testing.T) {
		t.Parallel()

		tlsStore := repocreds.NewStore(nil, []repocreds.RepoCred{
			{
				URL:     "https://secure.example.com",
				TLSCert: []byte("cert-data"),
				TLSKey:  []byte("key-data"),
			},
		})

		c := &Client{CredsStore: tlsStore}
		resolved := c.credForURL("https://secure.example.com/chart")

		require.NotEmpty(t, resolved.certFile)
		require.NotEmpty(t, resolved.keyFile)
		assert.Len(t, resolved.cleanups, 2)

		resolved.cleanup()

		// Files should be removed after cleanup
		assert.NoFileExists(t, resolved.certFile)
		assert.NoFileExists(t, resolved.keyFile)
	})
}

func TestOCILoginOpts(t *testing.T) {
	t.Parallel()

	t.Run("basic auth only", func(t *testing.T) {
		t.Parallel()

		cred := &resolvedCred{Username: "user", Password: "pass"}
		opts := ociLoginOpts(cred)

		assert.Len(t, opts, 1)
	})

	t.Run("TLS only", func(t *testing.T) {
		t.Parallel()

		cred := &resolvedCred{certFile: "/tmp/cert.pem", keyFile: "/tmp/key.pem"}
		opts := ociLoginOpts(cred)

		assert.Len(t, opts, 1)
	})

	t.Run("basic auth and TLS combined", func(t *testing.T) {
		t.Parallel()

		cred := &resolvedCred{
			Username: "user",
			Password: "pass",
			certFile: "/tmp/cert.pem",
			keyFile:  "/tmp/key.pem",
		}
		opts := ociLoginOpts(cred)

		assert.Len(t, opts, 2)
	})

	t.Run("no auth data returns empty", func(t *testing.T) {
		t.Parallel()

		cred := &resolvedCred{}
		opts := ociLoginOpts(cred)

		assert.Empty(t, opts)
	})

	t.Run("cert without key returns empty", func(t *testing.T) {
		t.Parallel()

		cred := &resolvedCred{certFile: "/tmp/cert.pem"}
		opts := ociLoginOpts(cred)

		assert.Empty(t, opts)
	})

	t.Run("key without cert returns empty", func(t *testing.T) {
		t.Parallel()

		cred := &resolvedCred{keyFile: "/tmp/key.pem"}
		opts := ociLoginOpts(cred)

		assert.Empty(t, opts)
	})

	t.Run("username without password still produces basic auth", func(t *testing.T) {
		t.Parallel()

		cred := &resolvedCred{Username: "user"}
		opts := ociLoginOpts(cred)

		assert.Len(t, opts, 1)
	})
}

func TestWriteTempPEM(t *testing.T) {
	t.Parallel()

	t.Run("writes data and returns valid path", func(t *testing.T) {
		t.Parallel()

		path := writeTempPEM([]byte("test-pem-data"))
		require.NotEmpty(t, path)

		defer os.Remove(path)

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, []byte("test-pem-data"), content)

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})

	t.Run("empty data writes empty file", func(t *testing.T) {
		t.Parallel()

		path := writeTempPEM([]byte{})
		require.NotEmpty(t, path)

		defer os.Remove(path)

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Empty(t, content)
	})
}
