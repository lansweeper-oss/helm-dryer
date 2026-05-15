package client //nolint:testpackage // tests unexported helpers

import (
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
