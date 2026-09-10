package dryer

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/lansweeper-oss/helm-dryer/internal/argo"
	dryerr "github.com/lansweeper-oss/helm-dryer/internal/errors"
	"github.com/lansweeper-oss/helm-dryer/internal/utils"
)

// repoRoot returns the folder used as a base to resolve absolute values files paths.
// When unknown, it falls back to the chart path, which makes absolute and relative entries
// equivalent as there is no repository to speak of (for example, a local `template` run).
func (in *Input) repoRoot() string {
	if in.Settings.RepoRoot != "" {
		return in.Settings.RepoRoot
	}

	return in.Settings.Path
}

// resolveRepoRootFromEnv derives the repository root from the environment, unless it was
// explicitly set as a parameter.
//
// ArgoCD does not expose the repository root to a CMP plugin, but the sidecar streams a
// tarball of (part of) the repository and runs the plugin from the application folder.
// Trimming the source path (ARGOCD_APP_SOURCE_PATH, relative to the repository root) from
// the chart path yields the root.
//
// When the argocd.argoproj.io/manifest-generate-paths annotation narrows the tarball to a
// subdirectory of the repository, the chart path no longer ends with the full source path.
// A partial suffix match detects this and records the trimmed prefix so that absolute values
// files can be resolved correctly.
func (in *Input) resolveRepoRootFromEnv() {
	if in.Settings.RepoRoot != "" {
		return
	}

	sourcePath := utils.GetEnv(argo.SourcePath, "")

	result, err := repoRootFromSourcePath(in.Settings.Path, sourcePath)
	if err != nil {
		slog.Warn(
			"Could not determine the repository root, absolute values files will resolve from the chart path",
			"path", in.Settings.Path,
			"sourcePath", sourcePath,
			"error", err,
		)

		return
	}

	if result.trimmedPrefix != "" {
		slog.Info(
			"Repository root resolved with a trimmed prefix (manifest-generate-paths tarball narrowing detected)",
			"repoRoot", result.root,
			"trimmedPrefix", result.trimmedPrefix,
			"sourcePath", sourcePath,
		)

		in.repoRootPrefix = result.trimmedPrefix
	} else {
		slog.Debug("Repository root resolved from the environment", "repoRoot", result.root, "sourcePath", sourcePath)
	}

	in.Settings.RepoRoot = result.root
}

// repoRootResult holds the outcome of resolving the repository root from the source path.
type repoRootResult struct {
	root string
	// trimmedPrefix is the leading portion of sourcePath that was absent from the chart path.
	// This happens when ArgoCD's manifest-generate-paths annotation causes the tarball root
	// to be a subdirectory of the repository, so the CMP working directory no longer contains
	// the full source path. Absolute values files must have this prefix stripped before
	// resolution.
	trimmedPrefix string
}

// repoRootFromSourcePath removes the repository-relative sourcePath suffix from chartPath.
// An empty (or dot) sourcePath means the application lives at the root of the repository.
//
// When the full suffix does not match, progressively shorter suffixes are tried. A partial
// match sets trimmedPrefix to the leading segments that were absent from the chart path,
// which callers use to adjust absolute values file resolution.
//
// When no suffix matches at all, the tarball was narrowed to the application directory
// itself (the annotation resolved to the application path), the extraction directory is the
// chart path, and the whole source path becomes the trimmed prefix.
func repoRootFromSourcePath(chartPath, sourcePath string) (repoRootResult, error) {
	absChartPath, err := filepath.Abs(chartPath)
	if err != nil {
		return repoRootResult{}, fmt.Errorf("failed to resolve chart path %s: %w", chartPath, err)
	}

	separator := string(os.PathSeparator)

	cleanSource := filepath.Clean(sourcePath)

	// an application sitting at the root of the repository has nothing to trim
	if cleanSource == "." || cleanSource == "" {
		return repoRootResult{root: absChartPath}, nil
	}

	segments := strings.Split(cleanSource, separator)

	// try each suffix from longest (full sourcePath) to shortest (last segment only)
	for i := range segments {
		candidate := separator + filepath.Join(segments[i:]...)

		root, trimmed := strings.CutSuffix(absChartPath, candidate)
		if !trimmed {
			continue
		}

		if root == "" {
			root = separator
		}

		result := repoRootResult{root: root}

		if i > 0 {
			result.trimmedPrefix = filepath.Join(segments[:i]...)
		}

		return result, nil
	}

	// no suffix of the source path matches: the tarball root is the application directory
	// itself, so its extraction directory (an arbitrary temporary name) is both the chart
	// path and the effective repository root
	return repoRootResult{root: absChartPath, trimmedPrefix: filepath.Join(segments...)}, nil
}

// resolveValuesFile resolves a values file entry passed to the plugin, mirroring how ArgoCD
// resolves spec.source.helm.valueFiles for native Helm sources: relative paths are resolved
// from the chart path, while absolute ones are resolved from the repository root.
//
// As ArgoCD does, an absolute entry may not escape the repository root. Relative entries are
// deliberately left unchecked to preserve the ability to reference files above the chart path.
func (in *Input) resolveValuesFile(file string) (string, error) {
	if !filepath.IsAbs(file) {
		return filepath.Join(in.Settings.Path, file), nil
	}

	root := in.repoRoot()
	adjusted := in.stripRepoRootPrefix(file)
	resolved := filepath.Join(root, adjusted)

	// a path we cannot relate to the root is one we cannot prove to be inside it either,
	// so both the unrelatable and the escaping paths are rejected
	relative, err := filepath.Rel(root, resolved)
	if err != nil || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("%w: %s (root: %s)", dryerr.ErrOutsideRepoRoot, file, root)
	}

	return resolved, nil
}

// stripRepoRootPrefix removes the leading directory prefix that was trimmed from the tarball
// when ArgoCD's manifest-generate-paths narrowed the transmitted tree. Without this, absolute
// values files like /security/_values/foo.yaml cannot resolve because the tarball root already
// represents the security/ directory.
func (in *Input) stripRepoRootPrefix(file string) string {
	if in.repoRootPrefix == "" {
		return file
	}

	prefix := string(os.PathSeparator) + in.repoRootPrefix + string(os.PathSeparator)

	after, found := strings.CutPrefix(file, prefix)
	if found {
		return string(os.PathSeparator) + after
	}

	return file
}
