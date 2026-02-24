package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
	"github.com/lansweeper/helm-dryer/internal/cli"
	"github.com/lansweeper/helm-dryer/internal/dryer"
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

	switch ctx.Command() {
	case "get":
		slog.Debug("Rendering values")

		dryer := dryer.Input{
			Data:     c.Data,
			Settings: c.Settings,
		}

		err := dryer.TemplateValues()
		if err != nil {
			return fmt.Errorf("failed to render values: %w", err)
		}

		return nil
	case "render-app":
		slog.Debug("Rendering chart with Dryer from an ArgoCD Application file")

		dryer := dryer.Input{
			AppSettings: c.RenderApp,
			Data:        c.Data,
			Settings:    c.Settings,
		}

		err := dryer.RenderFromApp()
		if err != nil {
			return fmt.Errorf("failed to render from app: %w", err)
		}

		return nil
	case "template":
		slog.Debug("Rendering chart with Dryer")

		dryer := dryer.Input{
			AppSettings: c.Template,
			Data:        c.Data,
			Settings:    c.Settings,
		}

		err := dryer.TemplateChart()
		if err != nil {
			return fmt.Errorf("failed to template chart: %w", err)
		}

		return nil
	case "render":
		dryer := dryer.Input{
			Data:     c.Data,
			Settings: c.Settings,
		}

		err := dryer.RenderChart()
		if err != nil {
			return fmt.Errorf("failed to render chart: %w", err)
		}

		return nil
	default: // show version
		// version is taken from environment variable in build time
		slog.Info("Helm-dryer version", "version", BuildVersion, "build_time", BuildTime)

		return nil
	}
}

func initLogger(debug bool, format string) {
	lvl := new(slog.LevelVar)
	if debug {
		lvl.Set(slog.LevelDebug)
	} else {
		lvl.Set(slog.LevelInfo)
	}

	var handler slog.Handler

	loggerOptions := &slog.HandlerOptions{
		Level: lvl,
	}

	if format == "console" {
		handler = slog.NewTextHandler(os.Stderr, loggerOptions)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, loggerOptions)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}
