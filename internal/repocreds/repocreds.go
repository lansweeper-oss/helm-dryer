// Package repocreds resolves ArgoCD repository credentials by URL matching.
package repocreds

import (
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
type Store struct {
	repos     []RepoCred
	templates []RepoCred
}

// NewStore creates a Store from repository secrets (exact match) and repo-creds templates (prefix match).
func NewStore(repos, templates []RepoCred) *Store {
	return &Store{repos: repos, templates: templates}
}

// Repos returns the repository credentials (exact match entries).
func (s *Store) Repos() []RepoCred { return s.repos }

// Templates returns the repo-creds template credentials (prefix match entries).
func (s *Store) Templates() []RepoCred { return s.templates }

// ForURL returns the best credential match for the given repository URL.
// Repository secrets (exact match) take precedence over repo-creds templates (longest prefix).
// Returns nil when no match is found.
func (s *Store) ForURL(repoURL string) *RepoCred {
	if s == nil {
		return nil
	}

	norm := normalizeURL(repoURL)

	for i := range s.repos {
		if normalizeURL(s.repos[i].URL) == norm {
			return &s.repos[i]
		}
	}

	best, bestLen := -1, 0

	for i := range s.templates {
		credURL := normalizeURL(s.templates[i].URL)
		if strings.HasPrefix(norm, credURL) && len(credURL) > bestLen {
			best, bestLen = i, len(credURL)
		}
	}

	if best >= 0 {
		return &s.templates[best]
	}

	return nil
}

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
