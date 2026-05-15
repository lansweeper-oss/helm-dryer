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
	"github.com/lansweeper-oss/helm-dryer/internal/repocreds"
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

func (c *CLI) newInput(ctx context.Context) dryer.Input {
	return dryer.Input{
		CredsStore: buildCredsStore(ctx, &c.Credentials),
		Data:       c.Data,
		Settings:   c.Settings,
	}
}

func buildCredsStore(ctx context.Context, creds *cli.Credentials) *repocreds.Store {
	var repos, templates []repocreds.RepoCred

	if creds.Username != "" && creds.Password != "" && creds.Registry != "" {
		repos = append(repos, repocreds.RepoCred{
			URL:      "oci://" + creds.Registry,
			Username: creds.Username,
			Password: creds.Password,
		})
	}

	if creds.Secret {
		store, err := repocreds.FetchFromCluster(ctx, creds.Namespace)
		if err != nil {
			slog.Warn("Failed to fetch ArgoCD credentials, continuing without them", "err", err)
		} else if store != nil {
			repos = append(repos, store.Repos()...)
			templates = append(templates, store.Templates()...)
		}
	}

	if len(repos) == 0 && len(templates) == 0 {
		return nil
	}

	return repocreds.NewStore(repos, templates)
}

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
