// Package app owns service lifecycle and background worker coordination.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/tiancheng91/mihomo-exporter/internal/collector"
	"github.com/tiancheng91/mihomo-exporter/internal/config"
	"github.com/tiancheng91/mihomo-exporter/internal/mihomo"
	promoutput "github.com/tiancheng91/mihomo-exporter/internal/prometheus"
	"github.com/tiancheng91/mihomo-exporter/internal/telegraf"
)

// Run blocks until cancellation or a server failure. All started workers are
// joined before it returns. Logging is local to this application instance.
func Run(parent context.Context, cfg config.Config, logger *slog.Logger, version, commit string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := parent.Err(); err != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	agg := collector.New(cfg.EnableClientProxy)
	client := mihomo.NewClient(cfg.Mihomo.URL, cfg.Mihomo.Secret, cfg.HTTP.Timeout)
	defer client.Close()
	var server *promoutput.Server
	var listener net.Listener
	if cfg.Prometheus.Enabled {
		var err error
		listener, err = net.Listen("tcp", cfg.Prometheus.Listen)
		if err != nil {
			return fmt.Errorf("listen for Prometheus: %w", err)
		}
		defer listener.Close()
		server = promoutput.NewServer(cfg.Prometheus.Listen, promoutput.NewExporter(agg, cfg.EnableClientProxy, version, commit))
	}
	var workers sync.WaitGroup
	start := func(work func()) { workers.Add(1); go func() { defer workers.Done(); work() }() }
	logger.Info("starting mihomo exporter", "version", version, "commit", commit, "prometheus", cfg.Prometheus.Enabled, "telegraf", cfg.Telegraf.Enabled)
	start(func() {
		reconnect(ctx, logger, "traffic", cfg.Mihomo.ReconnectInterval, agg, func(connected func()) error {
			return client.StreamTraffic(ctx, func(v mihomo.Traffic) { connected(); agg.ObserveTraffic(v) })
		})
	})
	start(func() {
		reconnect(ctx, logger, "connections", cfg.Mihomo.ReconnectInterval, agg, func(connected func()) error {
			return client.StreamConnections(ctx, cfg.Mihomo.ConnectionInterval, func(v mihomo.ConnectionsSnapshot) { connected(); agg.ObserveConnections(v, time.Now()) })
		})
	})
	if cfg.Telegraf.Enabled {
		p := telegraf.New(cfg.Telegraf.URL, cfg.Telegraf.FlushInterval, cfg.HTTP.Timeout, agg)
		start(func() {
			p.Run(ctx, func(err error) {
				if ctx.Err() == nil {
					logger.Warn("Telegraf push failed", "error", err)
				}
			})
		})
	}
	serverErrors := make(chan error, 1)
	if server != nil {
		start(func() { serverErrors <- server.Serve(listener) })
	}
	start(func() { summaries(ctx, logger, agg, time.Minute) })
	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-serverErrors:
	}
	cancel()
	logger.Info("shutting down")
	if server != nil {
		shutdownCtx, stop := context.WithTimeout(context.Background(), cfg.HTTP.Timeout)
		err := server.Shutdown(shutdownCtx)
		stop()
		if err != nil {
			_ = server.Close()
			runErr = errors.Join(runErr, fmt.Errorf("HTTP shutdown: %w", err))
		}
	}
	workers.Wait()
	return runErr
}

func reconnect(ctx context.Context, logger *slog.Logger, name string, delay time.Duration, agg *collector.Collector, stream func(func()) error) {
	defer agg.SetStreamConnected(name, false)
	for ctx.Err() == nil {
		var once sync.Once
		err := stream(func() {
			once.Do(func() { agg.SetStreamConnected(name, true); logger.Info("stream connected", "stream", name) })
		})
		agg.SetStreamConnected(name, false)
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, io.EOF) {
			logger.Info("stream disconnected", "stream", name)
		} else {
			logger.Warn("stream disconnected", "stream", name, "error", err)
		}
		logger.Info("reconnecting stream", "stream", name, "after", delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func summaries(ctx context.Context, logger *slog.Logger, agg *collector.Collector, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s := agg.Snapshot()
			logger.Info("periodic summary", "connections", s.Global.Active, "clients", len(s.Clients), "proxies", len(s.Proxies), "tracked_download_bytes", s.Tracked.Download, "tracking_ratio", s.SafeTrackingRatio())
		}
	}
}
