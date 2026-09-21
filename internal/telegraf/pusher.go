package telegraf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/tiancheng91/mihomo-exporter/internal/collector"
)

type Pusher struct {
	url       string
	interval  time.Duration
	collector *collector.Collector
	client    *http.Client
}

func New(url string, interval, timeout time.Duration, c *collector.Collector) *Pusher {
	return &Pusher{url: url, interval: interval, collector: c, client: &http.Client{Timeout: timeout}}
}
func NewWithClient(url string, interval time.Duration, c *collector.Collector, client *http.Client) *Pusher {
	return &Pusher{url: url, interval: interval, collector: c, client: client}
}

func (p *Pusher) Run(ctx context.Context, onError func(error)) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.PushOnce(ctx); err != nil {
				onError(err)
			}
		}
	}
}

func (p *Pusher) PushOnce(ctx context.Context) error {
	snapshot := p.collector.PendingSnapshot()
	body := Encode(snapshot)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, strings.NewReader(body))
	if err != nil {
		p.collector.RecordTelegrafError()
		return err
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := p.client.Do(req)
	if err != nil {
		p.collector.RecordTelegrafError()
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		p.collector.RecordTelegrafError()
		return fmt.Errorf("telegraf returned %s", resp.Status)
	}
	p.collector.CommitPending(snapshot)
	p.collector.RecordTelegrafSuccess(time.Now())
	return nil
}

func Encode(s collector.PendingSnapshot) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "mihomo_traffic upload_bytes=%di,download_bytes=%di,upload_rate=%di,download_rate=%di,active_connections=%di\n", s.Global.Upload, s.Global.Download, s.UploadRate, s.DownloadRate, s.Global.Active)
	clients := sortedStringKeys(s.Clients)
	for _, k := range clients {
		writeLine(&b, "mihomo_client", map[string]string{"client": k}, s.Clients[k])
	}
	proxies := sortedStringKeys(s.Proxies)
	for _, k := range proxies {
		writeLine(&b, "mihomo_proxy", map[string]string{"proxy": k}, s.Proxies[k])
	}
	pairs := make([]collector.Pair, 0, len(s.ClientProxies))
	for k := range s.ClientProxies {
		pairs = append(pairs, k)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Client == pairs[j].Client {
			return pairs[i].Proxy < pairs[j].Proxy
		}
		return pairs[i].Client < pairs[j].Client
	})
	for _, k := range pairs {
		writeLine(&b, "mihomo_client_proxy", map[string]string{"client": k.Client, "proxy": k.Proxy}, s.ClientProxies[k])
	}
	return b.String()
}

func writeLine(b *bytes.Buffer, measurement string, tags map[string]string, v collector.Aggregate) {
	b.WriteString(measurement)
	keys := sortedStringKeys(tags)
	for _, k := range keys {
		b.WriteByte(',')
		b.WriteString(escapeTag(k))
		b.WriteByte('=')
		b.WriteString(escapeTag(tags[k]))
	}
	fmt.Fprintf(b, " upload_bytes=%di,download_bytes=%di,active_connections=%di\n", v.Upload, v.Download, v.Active)
}
func escapeTag(v string) string {
	r := strings.NewReplacer("\\", "\\\\", " ", "\\ ", ",", "\\,", "=", "\\=")
	return r.Replace(v)
}
func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
