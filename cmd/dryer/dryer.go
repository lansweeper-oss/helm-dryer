package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"github.com/lansweeper-oss/helm-dryer/internal/cli"
	"github.com/lansweeper-oss/helm-dryer/internal/dryer"
	repoCredentials "github.com/lansweeper-oss/helm-dryer/internal/repocreds"
)

//nolint:gochecknoglobals
var (
	BuildTime    string
	BuildVersion string
)

// CLI represents the command-line interface for the application, and
// contains the options and flags that can be used when running the application.
type CLI struct {
	cli.Data
	cli.Settings

	Get       struct{}        `cmd:"" help:"Get the rendered values."`
	Render    cli.AppSettings `cmd:"" help:"Render the template as a Configuration Management plugin."`
	RenderApp cli.AppSettings `cmd:"" help:"Render from an ArgoCD Application file."`
	Template  cli.AppSettings `cmd:"" help:"Render the template."`
	Version   struct{}        `cmd:"" help:"Show version and quit."`
}

// Run calls the appropriate function based on the command provided.
func (c *CLI) Run(ctx *kong.Context) error {
	initLogger(c.Logging.Debug, c.Logging.Format)

	timeout, err := time.ParseDuration(c.Timeout)
	if err != nil {
		return fmt.Errorf("bad timeout: %w", err)
	}

	requestCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	input := c.newInput(requestCtx)

	var runErr error

	switch ctx.Command() {
	case "get":
		slog.Debug("Rendering values")

		runErr = input.TemplateValues(requestCtx)
	case "render-app":
		slog.Debug("Rendering chart with Dryer from an ArgoCD Application file")

		input.AppSettings = c.RenderApp
		runErr = input.RenderFromApp(requestCtx)
	case "template":
		slog.Debug("Rendering chart with Dryer")

		input.AppSettings = c.Template
		runErr = input.TemplateChart(requestCtx)
	case "render":
		runErr = input.RenderChart(requestCtx)
	default:
		slog.Info("Helm-dryer version", "version", BuildVersion, "build_time", BuildTime)

		return nil
	}

	if runErr != nil {
		return fmt.Errorf("%s: %w", ctx.Command(), runErr)
	}

	return nil
}

// newInput builds a dryer.Input from CLI flags, including any resolved credentials.
func (c *CLI) newInput(ctx context.Context) dryer.Input {
	return dryer.Input{
		CredsStore: buildCredsStore(ctx, &c.Credentials),
		Data:       c.Data,
		Settings:   c.Settings,
	}
}

// buildCredsStore merges CLI credentials and optional Kubernetes secrets into a credential store.
func buildCredsStore(ctx context.Context, credentials *cli.Credentials) *repoCredentials.Store {
	var repos, templates []repoCredentials.RepoCred

	if credentials.Username != "" && credentials.Password != "" && credentials.Registry != "" {
		repos = append(repos, repoCredentials.RepoCred{
			URL:      repoCredentials.OCISchemePrefix + credentials.Registry,
			Username: credentials.Username,
			Password: credentials.Password,
		})
	}

	if credentials.Secret {
		k8sRepoCreds, k8sTplCreds, err := repoCredentials.FetchFromCluster(ctx, credentials.Namespace)
		if err != nil {
			slog.Warn("Failed to fetch ArgoCD credentials, continuing without them", "err", err)
		} else {
			repos = append(repos, k8sRepoCreds...)
			templates = append(templates, k8sTplCreds...)
		}
	}

	if len(repos) == 0 && len(templates) == 0 {
		return nil
	}

	return repoCredentials.NewStore(repos, templates)
}

// initLogger configures the default slog logger with the given debug level and output format.
func initLogger(debug bool, format string) {
	var level slog.Level
	if debug {
		level = slog.LevelDebug
	}

	var handler slog.Handler

	loggerOptions := &slog.HandlerOptions{
		Level: level,
	}

	if format == "console" {
		handler = slog.NewTextHandler(os.Stderr, loggerOptions)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, loggerOptions)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}
