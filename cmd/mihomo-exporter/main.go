package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/tiancheng91/mihomo-exporter/internal/app"
	"github.com/tiancheng91/mihomo-exporter/internal/config"
)

var version = "dev"
var commit = "unknown"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.LookupEnv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, lookupEnv func(string) (string, bool), stdout, stderr io.Writer) int {
	cfg, showVersion, err := config.Parse(args, lookupEnv, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if showVersion {
		fmt.Fprintf(stdout, "mihomo-exporter %s (commit %s)\n", version, commit)
		return 0
	}
	var level slog.Level
	_ = level.UnmarshalText([]byte(cfg.LogLevel))
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))
	if err := app.Run(ctx, cfg, logger, version, commit); err != nil {
		logger.Error("exporter stopped", "error", err)
		return 1
	}
	return 0
}
