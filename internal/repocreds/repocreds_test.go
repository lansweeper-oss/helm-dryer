package repocreds_test

import (
	"testing"

	"github.com/lansweeper-oss/helm-dryer/internal/repocreds"
	"github.com/stretchr/testify/assert"
)

func TestForURL(t *testing.T) {
	t.Parallel()

	repos := []repocreds.RepoCred{
		{URL: "https://charts.example.com", Username: "repo-user", Password: "repo-pass"},
	}
	templates := []repocreds.RepoCred{
		{URL: "https://charts.example.com/prefix", Username: "prefix-user", Password: "prefix-pass"},
		{URL: "https://charts.example.com", Username: "tmpl-user", Password: "tmpl-pass"},
		{URL: "oci://ghcr.io/org", Username: "oci-user", Password: "oci-pass"},
	}
	store := repocreds.NewStore(repos, templates)

	tests := []struct {
		name         string
		repoURL      string
		wantUsername string
		wantNil      bool
	}{
		{
			name:         "exact match returns repository secret (not template)",
			repoURL:      "https://charts.example.com",
			wantUsername: "repo-user",
		},
		{
			name:         "prefix match returns longest prefix",
			repoURL:      "https://charts.example.com/prefix/charts",
			wantUsername: "prefix-user",
		},
		{
			name:         "shorter prefix used when no longer match",
			repoURL:      "https://charts.example.com/other",
			wantUsername: "tmpl-user",
		},
		{
			name:         "OCI prefix match",
			repoURL:      "oci://ghcr.io/org/mychart",
			wantUsername: "oci-user",
		},
		{
			name:    "no match returns nil",
			repoURL: "https://unknown.example.com/charts",
			wantNil: true,
		},
		{
			name:         "trailing slash normalized — matches without slash",
			repoURL:      "https://charts.example.com/",
			wantUsername: "repo-user",
		},
		{
			name:         "scheme+host case insensitive",
			repoURL:      "HTTPS://CHARTS.EXAMPLE.COM",
			wantUsername: "repo-user",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := store.ForURL(testCase.repoURL)
			if testCase.wantNil {
				assert.Nil(t, got)

				return
			}

			if assert.NotNil(t, got) {
				assert.Equal(t, testCase.wantUsername, got.Username)
			}
		})
	}
}

func TestForURL_OCIPrefixFallback(t *testing.T) {
	t.Parallel()

	repos := []repocreds.RepoCred{
		{URL: "oci://948285518623.dkr.ecr.eu-west-1.amazonaws.com", Username: "ecr-user"},
		{URL: "oci://ghcr.io/org/charts", Username: "ghcr-specific"},
		{URL: "https://charts.example.com", Username: "http-user"},
	}
	store := repocreds.NewStore(repos, nil)

	tests := []struct {
		name         string
		repoURL      string
		wantUsername string
		wantNil      bool
	}{
		{
			name:         "OCI repo secret matches dep with subpath",
			repoURL:      "oci://948285518623.dkr.ecr.eu-west-1.amazonaws.com/helm",
			wantUsername: "ecr-user",
		},
		{
			name:         "OCI repo secret matches dep with deeper subpath",
			repoURL:      "oci://948285518623.dkr.ecr.eu-west-1.amazonaws.com/helm/app-resources",
			wantUsername: "ecr-user",
		},
		{
			name:         "OCI exact match still takes precedence",
			repoURL:      "oci://ghcr.io/org/charts",
			wantUsername: "ghcr-specific",
		},
		{
			name:         "OCI reverse prefix — dep is parent of cred",
			repoURL:      "oci://ghcr.io/org",
			wantUsername: "ghcr-specific",
		},
		{
			name:    "HTTP repo secret does NOT prefix-match",
			repoURL: "https://charts.example.com/subpath",
			wantNil: true,
		},
		{
			name:    "OCI no match on different host",
			repoURL: "oci://other-registry.example.com/charts",
			wantNil: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := store.ForURL(testCase.repoURL)
			if testCase.wantNil {
				assert.Nil(t, got)

				return
			}

			if assert.NotNil(t, got) {
				assert.Equal(t, testCase.wantUsername, got.Username)
			}
		})
	}
}

func TestForURL_OCIPrefixFallbackPrecedence(t *testing.T) {
	t.Parallel()

	repos := []repocreds.RepoCred{
		{URL: "oci://registry.example.com", Username: "short"},
		{URL: "oci://registry.example.com/org", Username: "long"},
	}
	store := repocreds.NewStore(repos, nil)

	got := store.ForURL("oci://registry.example.com/org/chart")
	if assert.NotNil(t, got) {
		assert.Equal(t, "long", got.Username, "longest OCI prefix should win")
	}
}

func TestForURL_TemplateTakesPrecedenceOverOCIFallback(t *testing.T) {
	t.Parallel()

	repos := []repocreds.RepoCred{
		{URL: "oci://registry.example.com", Username: "repo-user"},
	}
	templates := []repocreds.RepoCred{
		{URL: "oci://registry.example.com/org", Username: "tmpl-user"},
	}
	store := repocreds.NewStore(repos, templates)

	got := store.ForURL("oci://registry.example.com/org/chart")
	if assert.NotNil(t, got) {
		assert.Equal(t, "tmpl-user", got.Username, "template prefix match should win over OCI repo fallback")
	}
}

func TestForURL_NilStore(t *testing.T) {
	t.Parallel()

	var s *repocreds.Store
	assert.Nil(t, s.ForURL("https://charts.example.com"))
}

func TestForURL_EmptyStore(t *testing.T) {
	t.Parallel()

	store := repocreds.NewStore(nil, nil)
	assert.Nil(t, store.ForURL("https://charts.example.com"))
}
