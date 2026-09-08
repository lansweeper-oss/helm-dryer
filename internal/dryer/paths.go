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
// ArgoCD does not expose the repository root to a CMP plugin, but the sidecar unpacks the whole
// repository and runs the plugin from the application folder, so trimming the source path
// (ARGOCD_APP_SOURCE_PATH, relative to the repository root) from the chart path yields the root.
func (in *Input) resolveRepoRootFromEnv() {
	if in.Settings.RepoRoot != "" {
		return
	}

	sourcePath := utils.GetEnv(argo.SourcePath, "")

	root, err := repoRootFromSourcePath(in.Settings.Path, sourcePath)
	if err != nil {
		slog.Warn(
			"Could not determine the repository root, absolute values files will resolve from the chart path",
			"path", in.Settings.Path,
			"sourcePath", sourcePath,
			"error", err,
		)

		return
	}

	slog.Debug("Repository root resolved from the environment", "repoRoot", root, "sourcePath", sourcePath)

	in.Settings.RepoRoot = root
}

// repoRootFromSourcePath removes the repository-relative sourcePath suffix from chartPath.
// An empty (or dot) sourcePath means the application lives at the root of the repository.
func repoRootFromSourcePath(chartPath, sourcePath string) (string, error) {
	absChartPath, err := filepath.Abs(chartPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve chart path %s: %w", chartPath, err)
	}

	separator := string(os.PathSeparator)

	suffix := filepath.Clean(separator + sourcePath)

	// an application sitting at the root of the repository has nothing to trim
	if suffix == separator {
		return absChartPath, nil
	}

	root, trimmed := strings.CutSuffix(absChartPath, suffix)
	if !trimmed {
		return "", fmt.Errorf("%w: %s does not end with %s", dryerr.ErrSourcePathMismatch, absChartPath, suffix)
	}

	// the whole chart path was the source path, so the repository sits at the filesystem root
	if root == "" {
		root = separator
	}

	return root, nil
}

// resolveValuesFile resolves a values file entry the same way ArgoCD resolves the entries of
// spec.source.helm.valueFiles: relative paths are resolved from the chart path, while absolute
// ones are resolved from the repository root.
//
// As ArgoCD does, an absolute entry may not escape the repository root. Relative entries are
// deliberately left unchecked to preserve the ability to reference files above the chart path.
func (in *Input) resolveValuesFile(file string) (string, error) {
	if !filepath.IsAbs(file) {
		return filepath.Join(in.Settings.Path, file), nil
	}

	root := in.repoRoot()
	resolved := filepath.Join(root, file)

	// a path we cannot relate to the root is one we cannot prove to be inside it either,
	// so both the unrelatable and the escaping paths are rejected
	relative, err := filepath.Rel(root, resolved)
	if err != nil || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("%w: %s (root: %s)", dryerr.ErrOutsideRepoRoot, file, root)
	}

	return resolved, nil
}
