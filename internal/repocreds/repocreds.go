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

type normalizedCred struct {
	norm string
	cred *RepoCred
}

// Store matches repository URLs to credentials.
// Repository secrets use exact URL matching; repo-creds templates use longest-prefix matching.
// URLs are normalized at construction to avoid repeated parsing during lookups.
type Store struct {
	repos     []normalizedCred
	templates []normalizedCred
}

// NewStore creates a Store from repository secrets (exact match) and repo-creds templates (prefix match).
func NewStore(repos, templates []RepoCred) *Store {
	normalizedRepos := make([]normalizedCred, len(repos))
	for i := range repos {
		normalizedRepos[i] = normalizedCred{norm: normalizeURL(repos[i].URL), cred: &repos[i]}
	}

	normalizedTemplates := make([]normalizedCred, len(templates))
	for i := range templates {
		normalizedTemplates[i] = normalizedCred{norm: normalizeURL(templates[i].URL), cred: &templates[i]}
	}

	return &Store{
		repos:     normalizedRepos,
		templates: normalizedTemplates,
	}
}

// ForURL returns the best credential match for the given repository URL.
//
// Matching order (first match wins):
//  1. Exact match on repository secrets.
//  2. Longest-prefix match on repo-creds templates.
//  3. OCI fallback: bidirectional prefix match on OCI repository secrets,
//     mirroring ArgoCD's behavior for enableOCI repositories (see https://github.com/argoproj/argo-cd/issues/14636).
//
// Returns nil when no match is found.
func (s *Store) ForURL(repoURL string) *RepoCred {
	if s == nil {
		return nil
	}

	norm := normalizeURL(repoURL)

	for _, entry := range s.repos {
		if entry.norm == norm {
			slog.Debug("Credential matched (exact)", "url", repoURL, "secretURL", entry.cred.URL)

			return entry.cred
		}
	}

	best, bestLen := (*RepoCred)(nil), 0

	for _, entry := range s.templates {
		if strings.HasPrefix(norm, entry.norm) && len(entry.norm) > bestLen {
			best, bestLen = entry.cred, len(entry.norm)
		}
	}

	if best != nil {
		slog.Debug("Credential matched (prefix)", "url", repoURL, "matchedURL", best.URL)

		return best
	}

	if strings.HasPrefix(norm, "oci://") {
		for _, entry := range s.repos {
			if !strings.HasPrefix(entry.norm, "oci://") {
				continue
			}

			if (strings.HasPrefix(norm, entry.norm) || strings.HasPrefix(entry.norm, norm)) && len(entry.norm) > bestLen {
				best, bestLen = entry.cred, len(entry.norm)
			}
		}

		if best != nil {
			slog.Debug("Credential matched (OCI prefix)", "url", repoURL, "matchedURL", best.URL)

			return best
		}
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
