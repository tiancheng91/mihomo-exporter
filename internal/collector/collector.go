package collector

import (
	"math"
	"sync"
	"time"

	"github.com/tiancheng91/mihomo-exporter/internal/mihomo"
)

type Bytes struct{ Upload, Download uint64 }
type Aggregate struct {
	Bytes
	Active uint64
}
type Pair struct{ Client, Proxy string }
type ConnectionState struct {
	SourceIP, ProxyNode string
	Upload, Download    uint64
	LastSeen            time.Time
}

type Snapshot struct {
	Global                   Aggregate
	UploadRate, DownloadRate uint64
	Clients                  map[string]Aggregate
	Proxies                  map[string]Aggregate
	ClientProxies            map[Pair]Aggregate
	Tracked                  Bytes
	Missed                   Bytes
	TrackingRatio            float64
	StreamConnected          map[string]bool
	TelegrafPushErrors       uint64
	TelegrafLastSuccess      float64
}

type PendingSnapshot struct {
	Global                   Aggregate
	UploadRate, DownloadRate uint64
	Clients                  map[string]Aggregate
	Proxies                  map[string]Aggregate
	ClientProxies            map[Pair]Aggregate
}

type Collector struct {
	mu                       sync.RWMutex
	enableClientProxy        bool
	connections              map[string]ConnectionState
	global                   Aggregate
	uploadRate, downloadRate uint64
	clients                  map[string]Aggregate
	proxies                  map[string]Aggregate
	clientProxies            map[Pair]Aggregate
	pendingGlobal            Bytes
	pendingClients           map[string]Bytes
	pendingProxies           map[string]Bytes
	pendingClientProxies     map[Pair]Bytes
	tracked, missed          Bytes
	intervalTracked          Bytes
	trackingRatio            float64
	trafficBaseline          *Bytes
	streamConnected          map[string]bool
	telegrafPushErrors       uint64
	telegrafLastSuccess      float64
}

func New(enableClientProxy bool) *Collector {
	return &Collector{enableClientProxy: enableClientProxy, connections: map[string]ConnectionState{}, clients: map[string]Aggregate{}, proxies: map[string]Aggregate{}, clientProxies: map[Pair]Aggregate{}, pendingClients: map[string]Bytes{}, pendingProxies: map[string]Bytes{}, pendingClientProxies: map[Pair]Bytes{}, streamConnected: map[string]bool{"traffic": false, "connections": false}, trackingRatio: 1}
}

func ResolveProxyNode(conn mihomo.Connection) string {
	if len(conn.Chains) == 0 || conn.Chains[0] == "" {
		return "DIRECT"
	}
	return conn.Chains[0]
}

func (c *Collector) ObserveTraffic(v mihomo.Traffic) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.uploadRate, c.downloadRate = v.Up, v.Down
	current := Bytes{v.UpTotal, v.DownTotal}
	if c.trafficBaseline == nil {
		c.trafficBaseline = &current
		c.intervalTracked = Bytes{}
		return
	}
	previous := *c.trafficBaseline
	c.trafficBaseline = &current
	delta := Bytes{}
	if current.Upload >= previous.Upload {
		delta.Upload = current.Upload - previous.Upload
	}
	if current.Download >= previous.Download {
		delta.Download = current.Download - previous.Download
	}
	c.global.Upload += delta.Upload
	c.global.Download += delta.Download
	c.pendingGlobal.Upload += delta.Upload
	c.pendingGlobal.Download += delta.Download
	total := delta.Upload + delta.Download
	tracked := c.intervalTracked.Upload + c.intervalTracked.Download
	if total == 0 {
		if tracked == 0 {
			c.trackingRatio = 1
		} else {
			c.trackingRatio = 0
		}
	} else {
		c.trackingRatio = float64(tracked) / float64(total)
	}
	if delta.Upload > c.intervalTracked.Upload {
		c.missed.Upload += delta.Upload - c.intervalTracked.Upload
	}
	if delta.Download > c.intervalTracked.Download {
		c.missed.Download += delta.Download - c.intervalTracked.Download
	}
	c.intervalTracked = Bytes{}
}

func (c *Collector) ObserveConnections(snapshot mihomo.ConnectionsSnapshot, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := make(map[string]struct{}, len(snapshot.Connections))
	activeClients := map[string]uint64{}
	activeProxies := map[string]uint64{}
	activePairs := map[Pair]uint64{}
	for _, conn := range snapshot.Connections {
		if conn.ID == "" {
			continue
		}
		client, proxy := conn.Metadata.SourceIP, ResolveProxyNode(conn)
		seen[conn.ID] = struct{}{}
		activeClients[client]++
		activeProxies[proxy]++
		pair := Pair{client, proxy}
		if c.enableClientProxy {
			activePairs[pair]++
		}
		if prev, ok := c.connections[conn.ID]; ok {
			delta := Bytes{}
			if conn.Upload >= prev.Upload {
				delta.Upload = conn.Upload - prev.Upload
			}
			if conn.Download >= prev.Download {
				delta.Download = conn.Download - prev.Download
			}
			// Attribute bytes to the metadata in the current complete snapshot.
			c.addDelta(client, proxy, pair, delta)
		}
		c.connections[conn.ID] = ConnectionState{client, proxy, conn.Upload, conn.Download, now}
	}
	for id := range c.connections {
		if _, ok := seen[id]; !ok {
			delete(c.connections, id)
		}
	}
	c.global.Active = uint64(len(seen))
	setActive(c.clients, activeClients)
	setActive(c.proxies, activeProxies)
	if c.enableClientProxy {
		setPairActive(c.clientProxies, activePairs)
	}
}

func (c *Collector) addDelta(client, proxy string, pair Pair, delta Bytes) {
	c.tracked.Upload += delta.Upload
	c.tracked.Download += delta.Download
	c.intervalTracked.Upload += delta.Upload
	c.intervalTracked.Download += delta.Download
	addAggregate(c.clients, client, delta)
	addAggregate(c.proxies, proxy, delta)
	addBytes(c.pendingClients, client, delta)
	addBytes(c.pendingProxies, proxy, delta)
	if c.enableClientProxy {
		addAggregate(c.clientProxies, pair, delta)
		addBytes(c.pendingClientProxies, pair, delta)
	}
}

func addAggregate[K comparable](m map[K]Aggregate, key K, b Bytes) {
	v := m[key]
	v.Upload += b.Upload
	v.Download += b.Download
	m[key] = v
}
func addBytes[K comparable](m map[K]Bytes, key K, b Bytes) {
	v := m[key]
	v.Upload += b.Upload
	v.Download += b.Download
	m[key] = v
}
func setActive(m map[string]Aggregate, active map[string]uint64) {
	for k, v := range m {
		v.Active = active[k]
		m[k] = v
	}
	for k, n := range active {
		v := m[k]
		v.Active = n
		m[k] = v
	}
}
func setPairActive(m map[Pair]Aggregate, active map[Pair]uint64) {
	for k, v := range m {
		v.Active = active[k]
		m[k] = v
	}
	for k, n := range active {
		v := m[k]
		v.Active = n
		m[k] = v
	}
}

func (c *Collector) SetStreamConnected(stream string, connected bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.streamConnected[stream] = connected
}
func (c *Collector) RecordTelegrafError() { c.mu.Lock(); defer c.mu.Unlock(); c.telegrafPushErrors++ }
func (c *Collector) RecordTelegrafSuccess(at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.telegrafLastSuccess = float64(at.Unix())
}

func (c *Collector) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Snapshot{c.global, c.uploadRate, c.downloadRate, clone(c.clients), clone(c.proxies), clone(c.clientProxies), c.tracked, c.missed, c.trackingRatio, clone(c.streamConnected), c.telegrafPushErrors, c.telegrafLastSuccess}
}

func (c *Collector) PendingSnapshot() PendingSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p := PendingSnapshot{Global: Aggregate{Bytes: c.pendingGlobal, Active: c.global.Active}, UploadRate: c.uploadRate, DownloadRate: c.downloadRate, Clients: map[string]Aggregate{}, Proxies: map[string]Aggregate{}, ClientProxies: map[Pair]Aggregate{}}
	mergePending(p.Clients, c.pendingClients, c.clients)
	mergePending(p.Proxies, c.pendingProxies, c.proxies)
	if c.enableClientProxy {
		mergePairPending(p.ClientProxies, c.pendingClientProxies, c.clientProxies)
	}
	return p
}

func (c *Collector) CommitPending(sent PendingSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pendingGlobal = subtract(c.pendingGlobal, sent.Global.Bytes)
	commitMap(c.pendingClients, sent.Clients)
	commitMap(c.pendingProxies, sent.Proxies)
	if c.enableClientProxy {
		commitPairMap(c.pendingClientProxies, sent.ClientProxies)
	}
}

func mergePending(dst map[string]Aggregate, pending map[string]Bytes, totals map[string]Aggregate) {
	for k, b := range pending {
		dst[k] = Aggregate{Bytes: b}
	}
	for k, v := range totals {
		x := dst[k]
		x.Active = v.Active
		dst[k] = x
	}
}
func mergePairPending(dst map[Pair]Aggregate, pending map[Pair]Bytes, totals map[Pair]Aggregate) {
	for k, b := range pending {
		dst[k] = Aggregate{Bytes: b}
	}
	for k, v := range totals {
		x := dst[k]
		x.Active = v.Active
		dst[k] = x
	}
}
func subtract(a, b Bytes) Bytes {
	return Bytes{saturatingSub(a.Upload, b.Upload), saturatingSub(a.Download, b.Download)}
}
func saturatingSub(a, b uint64) uint64 {
	if b > a {
		return 0
	}
	return a - b
}
func commitMap(pending map[string]Bytes, sent map[string]Aggregate) {
	for k, v := range sent {
		next := subtract(pending[k], v.Bytes)
		if next == (Bytes{}) {
			delete(pending, k)
		} else {
			pending[k] = next
		}
	}
}
func commitPairMap(pending map[Pair]Bytes, sent map[Pair]Aggregate) {
	for k, v := range sent {
		next := subtract(pending[k], v.Bytes)
		if next == (Bytes{}) {
			delete(pending, k)
		} else {
			pending[k] = next
		}
	}
}
func clone[K comparable, V any](src map[K]V) map[K]V {
	dst := make(map[K]V, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (s Snapshot) SafeTrackingRatio() float64 {
	if math.IsNaN(s.TrackingRatio) || math.IsInf(s.TrackingRatio, 0) {
		return 0
	}
	return s.TrackingRatio
}
