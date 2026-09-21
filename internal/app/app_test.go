package app

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/tiancheng91/mihomo-exporter/internal/config"
	"github.com/tiancheng91/mihomo-exporter/internal/mihomo"
)

func TestListenFailureReturnsError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg := config.Defaults()
	cfg.Prometheus.Listen = listener.Addr().String()
	if err := Run(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", "test"); err == nil {
		t.Fatal("expected bind failure")
	}
}

func TestCancellationStopsStreamsAndRun(t *testing.T) {
	started := make(chan struct{}, 2)
	stopped := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		if r.URL.Path == "/connections" {
			_ = wsjson.Write(r.Context(), conn, mihomo.ConnectionsSnapshot{})
		} else {
			_ = wsjson.Write(r.Context(), conn, mihomo.Traffic{})
		}
		started <- struct{}{}
		_, _, _ = conn.Read(context.Background())
		stopped <- struct{}{}
	}))
	defer server.Close()
	cfg := config.Defaults()
	cfg.Mihomo.URL = server.URL
	cfg.Prometheus.Listen = "127.0.0.1:0"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", "test") }()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("stream did not start")
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
	for i := 0; i < 2; i++ {
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Fatal("stream did not stop")
		}
	}
}
