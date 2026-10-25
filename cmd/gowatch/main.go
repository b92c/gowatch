package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/b92c/gowatch/internal/aws"
	"github.com/b92c/gowatch/internal/config"
	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/internal/trace"
	"github.com/b92c/gowatch/internal/ui"
	"github.com/moby/moby/client"
)

// otlpShutdownTimeout bounds how long the OTLP receiver may take to stop.
const otlpShutdownTimeout = 2 * time.Second

type cliOptions struct {
	configPath         string
	theme              string
	printDefaultConfig bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// parseFlags reads the command line. The flag package reports problems and
// usage on stderr.
func parseFlags(args []string, stderr io.Writer) (cliOptions, error) {
	var opts cliOptions

	fs := flag.NewFlagSet("gowatch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.configPath, "config", "", "path to a config file; replaces ~/.gowatch/config.yaml and ./gowatch.yaml")
	fs.StringVar(&opts.theme, "theme", "", "color theme ("+strings.Join(ui.ThemeNames(), ", ")+"); overrides theme.name from the config file")
	fs.BoolVar(&opts.printDefaultConfig, "print-default-config", false, "print a commented example configuration and exit")
	if err := fs.Parse(args); err != nil {
		return cliOptions{}, err
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "gowatch: unexpected argument %q\n", fs.Arg(0))
		fs.Usage()
		return cliOptions{}, errors.New("unexpected arguments")
	}
	return opts, nil
}

// themeFor resolves the theme to use. The --theme flag, when set, replaces
// theme.name from the config file; the colors under theme.colors still apply
// on top of whichever theme is chosen.
func themeFor(cfg config.ThemeConfig, flagName string) (ui.Theme, error) {
	if flagName != "" {
		cfg.Name = flagName
	}
	return ui.ResolveTheme(cfg)
}

// run is main without the process exit, so deferred cleanup always happens. It
// returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if opts.printDefaultConfig {
		if _, err := stdout.Write(config.DefaultYAML()); err != nil {
			fmt.Fprintf(stderr, "gowatch: %v\n", err)
			return 1
		}
		return 0
	}

	// Without a home directory the ~/.gowatch/config.yaml layer is simply
	// skipped (an empty HomeDir), so the lookup error is not worth failing for.
	homeDir, _ := os.UserHomeDir()
	workDir, err := os.Getwd()
	if err != nil {
		workDir = "."
	}
	cfg, _, err := config.Load(config.LoadOptions{
		ExplicitPath: opts.configPath,
		HomeDir:      homeDir,
		WorkDir:      workDir,
	})
	if err != nil {
		fmt.Fprintf(stderr, "gowatch: %v\n", err)
		return 1
	}

	theme, err := themeFor(cfg.Theme, opts.theme)
	if err != nil {
		fmt.Fprintf(stderr, "gowatch: %v\n", err)
		return 1
	}

	keymap, err := ui.BuildKeymap(cfg.Keybindings)
	if err != nil {
		fmt.Fprintf(stderr, "gowatch: %v\n", err)
		return 1
	}

	ctx := context.Background()
	apiClient, err := client.New(client.FromEnv)
	if err != nil {
		fmt.Fprintf(stderr, "gowatch: failed to create Docker client: %v\n", err)
		return 1
	}
	defer apiClient.Close()

	docker.Configure(docker.Options{
		LogTailLines:     cfg.Logs.TailLines,
		HistoryMaxPoints: cfg.History.MaxPoints,
		AlertRules:       cfg.Alerts.Rules(),
		AWS: aws.Options{
			Region:      cfg.AWS.Region,
			EndpointURL: cfg.AWS.EndpointURL,
			Enabled:     cfg.AWS.EnabledOverride(),
		},
	})

	if cfg.Tracing.OTLPReceiver.Enabled {
		receiver := trace.NewOTLPReceiver(cfg.Tracing.OTLPReceiver.Address, docker.GetGlobalTraceStore())
		if err := receiver.Start(); err != nil {
			fmt.Fprintf(stderr, "gowatch: tracing.otlp_receiver: %v\n", err)
			return 1
		}
		defer func() {
			stopCtx, cancel := context.WithTimeout(context.Background(), otlpShutdownTimeout)
			defer cancel()
			_ = receiver.Stop(stopCtx)
		}()
	}

	dashboard := ui.NewDashboardWithOptions(ui.Options{Theme: &theme, Keymap: &keymap, Layout: cfg.Layout})
	dashboard.ApplyInitialFilters(cfg.Filters.Search, cfg.Filters.Status, cfg.Filters.MinLogLevel())

	go func() {
		ticker := time.NewTicker(cfg.RefreshInterval)
		defer ticker.Stop()

		containers, err := docker.WatchContainers(ctx, apiClient)
		if err != nil {
			dashboard.ShowDaemonError(err)
		} else {
			dashboard.Update(containers)
		}

		for {
			select {
			case <-ticker.C:
				containers, err := docker.WatchContainers(ctx, apiClient)
				if err != nil {
					dashboard.ShowDaemonError(err)
					continue
				}
				dashboard.ClearDaemonError()
				dashboard.Update(containers)
			case <-ctx.Done():
				return
			}
		}
	}()

	if err := dashboard.Run(); err != nil {
		fmt.Fprintf(stderr, "gowatch: error running dashboard: %v\n", err)
		return 1
	}
	return 0
}
