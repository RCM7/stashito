package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/sethvargo/go-envconfig"
	"github.com/spf13/cobra"

	"github.com/RCM7/stashito/server/internal/app/server/config"
	"github.com/RCM7/stashito/server/internal/app/server/http/rest"
)

const configHint = "All variables are required: PORT, STORAGE_PATH, LOG_LEVEL, LOG_FORMAT, TAG_TTL and at least one UPSTREAM_<ALIAS>_HOST. Reference: https://stashito.com/llms.txt"

func setupLogger(level, format string) error {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return fmt.Errorf("LOG_LEVEL %q is invalid: use debug, info, warn or error", level)
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var handler slog.Handler
	switch format {
	case "text":
		handler = slog.NewTextHandler(os.Stderr, opts)
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		return fmt.Errorf("LOG_FORMAT %q is invalid: use text or json", format)
	}
	slog.SetDefault(slog.New(handler))
	return nil
}

var port int

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Run the Stashito API server",
	Long:  `The Stashito API server acts as a Docker image pull-through cache.`,
	Run: func(cmd *cobra.Command, args []string) {
		var c config.Config
		var err error

		ctx := context.Background()

		if err = envconfig.Process(ctx, &c); err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n%s\n", err, configHint)
			os.Exit(1)
		}

		if cmd.Flags().Changed("port") {
			c.Port = port
		}

		if err = setupLogger(c.LogLevel, c.LogFormat); err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n%s\n", err, configHint)
			os.Exit(1)
		}

		if c.Upstreams, err = config.ParseUpstreams(os.Environ()); err != nil {
			fmt.Fprintf(os.Stderr, "Error loading upstream config: %v\n%s\n", err, configHint)
			os.Exit(1)
		}

		fmt.Printf("Starting Stashito server on port %d...\n", c.Port)

		if err = rest.RunRouter(&c); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	apiCmd.PersistentFlags().IntVarP(&port, "port", "p", 8080, "Port that server will listen to")
}
