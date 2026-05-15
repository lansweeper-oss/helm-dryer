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
