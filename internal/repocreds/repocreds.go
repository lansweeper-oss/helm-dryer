// Package repocreds resolves ArgoCD repository credentials by URL matching.
package repocreds

import (
	"log/slog"
	"net/url"
	"strings"
)

// RepoCred holds credentials for a single repository or registry.
type RepoCred struct {
	URL      string
	Username string
	Password string
	TLSCert  []byte
	TLSKey   []byte
}

// Store matches repository URLs to credentials.
// Repository secrets use exact URL matching; repo-creds templates use longest-prefix matching.
// URLs are normalized at construction to avoid repeated parsing during lookups.
type Store struct {
	repos     []RepoCred
	templates []RepoCred
	normRepos []string
	normTmpls []string
}

// NewStore creates a Store from repository secrets (exact match) and repo-creds templates (prefix match).
func NewStore(repos, templates []RepoCred) *Store {
	normRepos := make([]string, len(repos))
	for i := range repos {
		normRepos[i] = normalizeURL(repos[i].URL)
	}

	normTmpls := make([]string, len(templates))
	for i := range templates {
		normTmpls[i] = normalizeURL(templates[i].URL)
	}

	return &Store{
		repos:     repos,
		templates: templates,
		normRepos: normRepos,
		normTmpls: normTmpls,
	}
}

// ForURL returns the best credential match for the given repository URL.
// Repository secrets (exact match) take precedence over repo-creds templates (longest prefix).
// Returns nil when no match is found.
func (s *Store) ForURL(repoURL string) *RepoCred {
	if s == nil {
		return nil
	}

	norm := normalizeURL(repoURL)

	for i, n := range s.normRepos {
		if n == norm {
			slog.Debug("Credential matched (exact)", "url", repoURL, "secretURL", s.repos[i].URL)

			return &s.repos[i]
		}
	}

	best, bestLen := -1, 0

	for i, n := range s.normTmpls {
		if strings.HasPrefix(norm, n) && len(n) > bestLen {
			best, bestLen = i, len(n)
		}
	}

	if best >= 0 {
		slog.Debug("Credential matched (prefix)", "url", repoURL, "templateURL", s.templates[best].URL)

		return &s.templates[best]
	}

	slog.Debug("No credential found", "url", repoURL)

	return nil
}

// normalizeURL canonicalizes a URL for comparison: trims trailing slashes, lowercases scheme and
// host. Path remains case-sensitive. This handles inconsistencies in how ArgoCD secrets store URLs
// (e.g. "https://example.com/Charts" vs "https://example.com/charts").
func normalizeURL(raw string) string {
	raw = strings.TrimRight(raw, "/")

	parsed, err := url.Parse(raw)
	if err != nil {
		return strings.ToLower(raw)
	}

	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Scheme = strings.ToLower(parsed.Scheme)

	return parsed.String()
}
