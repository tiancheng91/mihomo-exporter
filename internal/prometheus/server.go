package prometheus

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tiancheng91/mihomo-exporter/internal/collector"
)

type Exporter struct {
	collector         *collector.Collector
	enableClientProxy bool
	version, commit   string
	desc              map[string]*prometheus.Desc
}

func NewExporter(c *collector.Collector, enableClientProxy bool, version, commit string) *Exporter {
	d := func(name, help string, labels ...string) *prometheus.Desc {
		return prometheus.NewDesc(name, help, labels, nil)
	}
	return &Exporter{collector: c, enableClientProxy: enableClientProxy, version: version, commit: commit, desc: map[string]*prometheus.Desc{
		"up":            d("mihomo_upload_bytes_total", "Bytes uploaded according to the Mihomo traffic stream."),
		"down":          d("mihomo_download_bytes_total", "Bytes downloaded according to the Mihomo traffic stream."),
		"up_rate":       d("mihomo_upload_bytes_per_second", "Current upload rate reported by Mihomo."),
		"down_rate":     d("mihomo_download_bytes_per_second", "Current download rate reported by Mihomo."),
		"active":        d("mihomo_active_connections", "Current active connections."),
		"client_up":     d("mihomo_client_upload_bytes_total", "Tracked uploaded bytes by client.", "client"),
		"client_down":   d("mihomo_client_download_bytes_total", "Tracked downloaded bytes by client.", "client"),
		"client_active": d("mihomo_client_active_connections", "Current active connections by client.", "client"),
		"proxy_up":      d("mihomo_proxy_upload_bytes_total", "Tracked uploaded bytes by proxy.", "proxy"),
		"proxy_down":    d("mihomo_proxy_download_bytes_total", "Tracked downloaded bytes by proxy.", "proxy"),
		"proxy_active":  d("mihomo_proxy_active_connections", "Current active connections by proxy.", "proxy"),
		"pair_up":       d("mihomo_client_proxy_upload_bytes_total", "Tracked uploaded bytes by client and proxy.", "client", "proxy"),
		"pair_down":     d("mihomo_client_proxy_download_bytes_total", "Tracked downloaded bytes by client and proxy.", "client", "proxy"),
		"pair_active":   d("mihomo_client_proxy_active_connections", "Current active connections by client and proxy.", "client", "proxy"),
		"ratio":         d("mihomo_exporter_connection_tracking_ratio", "Ratio of connection-tracked bytes to traffic bytes in the latest traffic interval."),
		"tracked_up":    d("mihomo_exporter_tracked_upload_bytes_total", "Uploaded bytes observed through connection snapshots."),
		"tracked_down":  d("mihomo_exporter_tracked_download_bytes_total", "Downloaded bytes observed through connection snapshots."),
		"missed_up":     d("mihomo_exporter_missed_upload_bytes_total", "Traffic upload bytes not observed through connection snapshots."),
		"missed_down":   d("mihomo_exporter_missed_download_bytes_total", "Traffic download bytes not observed through connection snapshots."),
		"stream":        d("mihomo_exporter_stream_connected", "Whether a Mihomo stream is currently connected.", "stream"),
		"push_errors":   d("mihomo_exporter_telegraf_push_errors_total", "Failed Telegraf pushes."),
		"push_success":  d("mihomo_exporter_telegraf_last_success_timestamp_seconds", "Unix timestamp of the last successful Telegraf push."),
		"build":         d("mihomo_exporter_build_info", "Build information.", "version", "commit"),
	}}
}

func (*Exporter) Describe(chan<- *prometheus.Desc) {}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	s := e.collector.Snapshot()
	emit := func(key string, kind prometheus.ValueType, value float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(e.desc[key], kind, value, labels...)
	}
	emit("up", prometheus.CounterValue, float64(s.Global.Upload))
	emit("down", prometheus.CounterValue, float64(s.Global.Download))
	emit("up_rate", prometheus.GaugeValue, float64(s.UploadRate))
	emit("down_rate", prometheus.GaugeValue, float64(s.DownloadRate))
	emit("active", prometheus.GaugeValue, float64(s.Global.Active))
	for k, v := range s.Clients {
		emit("client_up", prometheus.CounterValue, float64(v.Upload), k)
		emit("client_down", prometheus.CounterValue, float64(v.Download), k)
		emit("client_active", prometheus.GaugeValue, float64(v.Active), k)
	}
	for k, v := range s.Proxies {
		emit("proxy_up", prometheus.CounterValue, float64(v.Upload), k)
		emit("proxy_down", prometheus.CounterValue, float64(v.Download), k)
		emit("proxy_active", prometheus.GaugeValue, float64(v.Active), k)
	}
	if e.enableClientProxy {
		for k, v := range s.ClientProxies {
			emit("pair_up", prometheus.CounterValue, float64(v.Upload), k.Client, k.Proxy)
			emit("pair_down", prometheus.CounterValue, float64(v.Download), k.Client, k.Proxy)
			emit("pair_active", prometheus.GaugeValue, float64(v.Active), k.Client, k.Proxy)
		}
	}
	emit("ratio", prometheus.GaugeValue, s.SafeTrackingRatio())
	emit("tracked_up", prometheus.CounterValue, float64(s.Tracked.Upload))
	emit("tracked_down", prometheus.CounterValue, float64(s.Tracked.Download))
	emit("missed_up", prometheus.CounterValue, float64(s.Missed.Upload))
	emit("missed_down", prometheus.CounterValue, float64(s.Missed.Download))
	for _, stream := range []string{"traffic", "connections"} {
		value := 0.0
		if s.StreamConnected[stream] {
			value = 1
		}
		emit("stream", prometheus.GaugeValue, value, stream)
	}
	emit("push_errors", prometheus.CounterValue, float64(s.TelegrafPushErrors))
	emit("push_success", prometheus.GaugeValue, s.TelegrafLastSuccess)
	emit("build", prometheus.GaugeValue, 1, e.version, e.commit)
}

type Server struct{ http *http.Server }

func NewServer(listen string, exporter *Exporter) *Server {
	registry := prometheus.NewRegistry()
	registry.MustRegister(exporter)
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	return &Server{http: &http.Server{Addr: listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}}
}

func (s *Server) ListenAndServe() error {
	err := s.http.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }
func (s *Server) Addr() string                       { return s.http.Addr }
func (s *Server) Close() error                       { return s.http.Close() }
func (s *Server) Serve(listener net.Listener) error {
	err := s.http.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
