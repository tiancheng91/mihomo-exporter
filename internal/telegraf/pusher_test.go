package telegraf

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tiancheng91/mihomo-exporter/internal/collector"
	"github.com/tiancheng91/mihomo-exporter/internal/mihomo"
)

func snapshot(upload, download uint64) mihomo.ConnectionsSnapshot {
	return mihomo.ConnectionsSnapshot{Connections: []mihomo.Connection{{ID: "1", Metadata: mihomo.Metadata{SourceIP: "client a,b=c\\d"}, Upload: upload, Download: download, Chains: []string{"proxy a,b=c\\d"}}}}
}

func TestPushFailureKeepsPending(t *testing.T) {
	c := collector.New(false)
	c.ObserveConnections(snapshot(0, 0), time.Now())
	c.ObserveConnections(snapshot(10, 20), time.Now())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusServiceUnavailable) }))
	defer server.Close()
	p := NewWithClient(server.URL, time.Second, c, server.Client())
	if err := p.PushOnce(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if got := c.PendingSnapshot().Clients["client a,b=c\\d"].Bytes; got != (collector.Bytes{Upload: 10, Download: 20}) {
		t.Fatalf("pending = %+v", got)
	}
	if c.Snapshot().TelegrafPushErrors != 1 {
		t.Fatal("push error counter not incremented")
	}
}

func TestConcurrentDeltaSurvivesSuccessfulPush(t *testing.T) {
	c := collector.New(false)
	c.ObserveConnections(snapshot(0, 0), time.Now())
	c.ObserveConnections(snapshot(10, 20), time.Now())
	started := make(chan struct{})
	release := make(chan struct{})
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	p := NewWithClient(server.URL, time.Second, c, server.Client())
	done := make(chan error, 1)
	go func() { done <- p.PushOnce(context.Background()) }()
	<-started
	c.ObserveConnections(snapshot(15, 27), time.Now())
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "upload_bytes=10i,download_bytes=20i") {
		t.Fatalf("body = %q", body)
	}
	if got := c.PendingSnapshot().Clients["client a,b=c\\d"].Bytes; got != (collector.Bytes{Upload: 5, Download: 7}) {
		t.Fatalf("pending = %+v", got)
	}
}

func TestInfluxTagEscaping(t *testing.T) {
	s := collector.PendingSnapshot{Global: collector.Aggregate{}, Clients: map[string]collector.Aggregate{"a b,c=d\\e": {}}, Proxies: map[string]collector.Aggregate{}, ClientProxies: map[collector.Pair]collector.Aggregate{}}
	got := Encode(s)
	if !strings.Contains(got, "client=a\\ b\\,c\\=d\\\\e") {
		t.Fatalf("escaped line missing in %q", got)
	}
}

func TestGlobalLineContainsRates(t *testing.T) {
	got := Encode(collector.PendingSnapshot{Global: collector.Aggregate{Bytes: collector.Bytes{Upload: 1, Download: 2}, Active: 3}, UploadRate: 4, DownloadRate: 5, Clients: map[string]collector.Aggregate{}, Proxies: map[string]collector.Aggregate{}, ClientProxies: map[collector.Pair]collector.Aggregate{}})
	if !strings.Contains(got, "upload_bytes=1i,download_bytes=2i,upload_rate=4i,download_rate=5i,active_connections=3i") {
		t.Fatal(got)
	}
}
