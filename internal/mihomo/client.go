package mihomo

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Traffic struct {
	Up        uint64 `json:"up"`
	Down      uint64 `json:"down"`
	UpTotal   uint64 `json:"upTotal"`
	DownTotal uint64 `json:"downTotal"`
}

type Connection struct {
	ID             string   `json:"id"`
	Metadata       Metadata `json:"metadata"`
	Upload         uint64   `json:"upload"`
	Download       uint64   `json:"download"`
	Chains         []string `json:"chains"`
	ProviderChains []string `json:"providerChains"`
}

type Metadata struct {
	SourceIP string `json:"sourceIP"`
}
type ConnectionsSnapshot struct {
	Connections []Connection `json:"connections"`
}

type Client struct {
	baseURL, secret string
	http            *http.Client
}

// Close releases idle transport connections after streaming workers have stopped.
func (c *Client) Close() { c.http.CloseIdleConnections() }

func NewClient(baseURL, secret string, timeout time.Duration) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext
	transport.ResponseHeaderTimeout = timeout
	transport.TLSHandshakeTimeout = timeout
	transport.IdleConnTimeout = timeout
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), secret: secret, http: &http.Client{Transport: transport}}
}

func (c *Client) StreamTraffic(ctx context.Context, receive func(Traffic)) error {
	return c.stream(ctx, "/traffic", nil, func(dec *json.Decoder) error {
		var v Traffic
		if err := dec.Decode(&v); err != nil {
			return err
		}
		receive(v)
		return nil
	})
}

func (c *Client) StreamConnections(ctx context.Context, interval time.Duration, receive func(ConnectionsSnapshot)) error {
	query := url.Values{"interval": {strconv.FormatInt(interval.Milliseconds(), 10)}}
	return c.stream(ctx, "/connections", query, func(dec *json.Decoder) error {
		var v ConnectionsSnapshot
		if err := dec.Decode(&v); err != nil {
			return err
		}
		receive(v)
		return nil
	})
}

func (c *Client) stream(ctx context.Context, path string, query url.Values, decode func(*json.Decoder) error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	if len(query) > 0 {
		req.URL.RawQuery = query.Encode()
	}
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s", path, resp.Status)
	}
	dec := json.NewDecoder(resp.Body)
	for {
		if err := decode(dec); err != nil {
			return err
		}
	}
}
