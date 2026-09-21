package prometheus

import (
	"strings"
	"testing"
	"time"

	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/tiancheng91/mihomo-exporter/internal/collector"
	"github.com/tiancheng91/mihomo-exporter/internal/mihomo"
)

func TestExporterExposesMetricsWithoutConsumingPending(t *testing.T) {
	c := collector.New(true)
	first := mihomo.ConnectionsSnapshot{Connections: []mihomo.Connection{{ID: "1", Metadata: mihomo.Metadata{SourceIP: "client"}, Chains: []string{"proxy"}}}}
	c.ObserveConnections(first, time.Now())
	second := first
	second.Connections[0].Upload = 7
	second.Connections[0].Download = 9
	c.ObserveConnections(second, time.Now())
	c.ObserveTraffic(mihomo.Traffic{UpTotal: 10, DownTotal: 20})
	c.ObserveTraffic(mihomo.Traffic{UpTotal: 17, DownTotal: 29, Up: 2, Down: 3})

	registry := prom.NewRegistry()
	registry.MustRegister(NewExporter(c, true, "v1", "abc"))
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for _, family := range families {
		if _, err := expfmt.MetricFamilyToText(&out, family); err != nil {
			t.Fatal(err)
		}
	}
	text := out.String()
	for _, name := range []string{
		"mihomo_upload_bytes_total", "mihomo_download_bytes_total",
		"mihomo_client_upload_bytes_total", "mihomo_proxy_download_bytes_total",
		"mihomo_client_proxy_active_connections", "mihomo_exporter_build_info",
	} {
		if !strings.Contains(text, name) {
			t.Errorf("metric %s not found", name)
		}
	}
	if got := c.PendingSnapshot().Clients["client"].Bytes; got != (collector.Bytes{Upload: 7, Download: 9}) {
		t.Fatalf("scrape consumed pending data: %+v", got)
	}
}
