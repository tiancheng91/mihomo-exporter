package collector

import (
	"testing"
	"time"

	"github.com/tiancheng91/mihomo-exporter/internal/mihomo"
)

func conn(id, client string, upload, download uint64, chains ...string) mihomo.Connection {
	return mihomo.Connection{ID: id, Metadata: mihomo.Metadata{SourceIP: client}, Upload: upload, Download: download, Chains: chains}
}
func observe(c *Collector, connections ...mihomo.Connection) {
	c.ObserveConnections(mihomo.ConnectionsSnapshot{Connections: connections}, time.Now())
}

func TestFirstConnectionEstablishesBaseline(t *testing.T) {
	c := New(false)
	observe(c, conn("1", "10.0.0.1", 100, 200, "HK"))
	s := c.Snapshot()
	if s.Tracked != (Bytes{}) || s.Global.Active != 1 {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
}

func TestConnectionDeltaAndAggregation(t *testing.T) {
	c := New(true)
	observe(c, conn("1", "10.0.0.1", 100, 200, "HK"))
	observe(c, conn("1", "10.0.0.1", 130, 280, "HK"))
	s := c.Snapshot()
	if s.Tracked != (Bytes{30, 80}) {
		t.Fatalf("tracked = %+v", s.Tracked)
	}
	if s.Clients["10.0.0.1"] != (Aggregate{Bytes: Bytes{30, 80}, Active: 1}) {
		t.Fatalf("client = %+v", s.Clients)
	}
	if s.Proxies["HK"] != (Aggregate{Bytes: Bytes{30, 80}, Active: 1}) {
		t.Fatalf("proxy = %+v", s.Proxies)
	}
	if s.ClientProxies[Pair{"10.0.0.1", "HK"}] != (Aggregate{Bytes: Bytes{30, 80}, Active: 1}) {
		t.Fatalf("pair = %+v", s.ClientProxies)
	}
}

func TestConnectionCounterReset(t *testing.T) {
	c := New(false)
	observe(c, conn("1", "c", 100, 200, "p"))
	observe(c, conn("1", "c", 10, 20, "p"))
	observe(c, conn("1", "c", 15, 27, "p"))
	if got := c.Snapshot().Tracked; got != (Bytes{5, 7}) {
		t.Fatalf("tracked = %+v", got)
	}
}

func TestConnectionDisappeared(t *testing.T) {
	c := New(false)
	observe(c, conn("1", "c", 1, 2, "p"))
	observe(c)
	if c.Snapshot().Global.Active != 0 || len(c.connections) != 0 {
		t.Fatal("connection was not removed")
	}
	if c.Snapshot().Clients["c"].Active != 0 || c.Snapshot().Proxies["p"].Active != 0 {
		t.Fatal("active gauges were not reset")
	}
	observe(c, conn("1", "c", 50, 60, "p"))
	if c.Snapshot().Tracked != (Bytes{}) {
		t.Fatal("reappearing connection must establish a new baseline")
	}
}

func TestResolveProxyNode(t *testing.T) {
	if got := ResolveProxyNode(conn("1", "c", 0, 0, "HK", "GLOBAL")); got != "HK" {
		t.Fatalf("got %q", got)
	}
	if got := ResolveProxyNode(conn("1", "c", 0, 0)); got != "DIRECT" {
		t.Fatalf("got %q", got)
	}
	if got := ResolveProxyNode(conn("1", "c", 0, 0, "")); got != "DIRECT" {
		t.Fatalf("got %q", got)
	}
}

func TestTrafficDeltaAndCounterReset(t *testing.T) {
	c := New(false)
	c.ObserveTraffic(mihomo.Traffic{UpTotal: 100, DownTotal: 200})
	c.ObserveTraffic(mihomo.Traffic{Up: 3, Down: 4, UpTotal: 130, DownTotal: 250})
	c.ObserveTraffic(mihomo.Traffic{UpTotal: 5, DownTotal: 10})
	c.ObserveTraffic(mihomo.Traffic{UpTotal: 8, DownTotal: 14})
	s := c.Snapshot()
	if s.Global.Bytes != (Bytes{33, 54}) {
		t.Fatalf("global = %+v", s.Global.Bytes)
	}
	if s.UploadRate != 0 || s.DownloadRate != 0 {
		t.Fatal("rates not updated")
	}
}

func TestPendingCommitOnlySubtractsSnapshot(t *testing.T) {
	c := New(false)
	observe(c, conn("1", "c", 0, 0, "p"))
	observe(c, conn("1", "c", 10, 20, "p"))
	first := c.PendingSnapshot()
	observe(c, conn("1", "c", 15, 27, "p"))
	c.CommitPending(first)
	pending := c.PendingSnapshot()
	if pending.Clients["c"].Bytes != (Bytes{5, 7}) || pending.Proxies["p"].Bytes != (Bytes{5, 7}) {
		t.Fatalf("new delta was cleared: %+v", pending)
	}
}

func TestTrackingAndMissedTraffic(t *testing.T) {
	c := New(false)
	c.ObserveTraffic(mihomo.Traffic{UpTotal: 100, DownTotal: 100})
	observe(c, conn("1", "c", 0, 0, "p"))
	observe(c, conn("1", "c", 20, 30, "p"))
	c.ObserveTraffic(mihomo.Traffic{UpTotal: 140, DownTotal: 160})
	s := c.Snapshot()
	if s.TrackingRatio != 0.5 || s.Missed != (Bytes{20, 30}) {
		t.Fatalf("ratio/missed = %v %+v", s.TrackingRatio, s.Missed)
	}
}
