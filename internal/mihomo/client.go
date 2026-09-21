package mihomo

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const maxStreamMessageSize = 32 << 20

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
	return c.stream(ctx, "/traffic", nil, func(conn *websocket.Conn) error {
		var v Traffic
		if err := wsjson.Read(ctx, conn, &v); err != nil {
			return err
		}
		receive(v)
		return nil
	})
}

func (c *Client) StreamConnections(ctx context.Context, interval time.Duration, receive func(ConnectionsSnapshot)) error {
	query := url.Values{"interval": {strconv.FormatInt(interval.Milliseconds(), 10)}}
	return c.stream(ctx, "/connections", query, func(conn *websocket.Conn) error {
		var v ConnectionsSnapshot
		if err := wsjson.Read(ctx, conn, &v); err != nil {
			return err
		}
		receive(v)
		return nil
	})
}

func (c *Client) stream(ctx context.Context, path string, query url.Values, read func(*websocket.Conn) error) error {
	endpoint, err := url.Parse(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("build %s URL: %w", path, err)
	}
	switch endpoint.Scheme {
	case "http":
		endpoint.Scheme = "ws"
	case "https":
		endpoint.Scheme = "wss"
	default:
		return fmt.Errorf("unsupported Mihomo URL scheme %q", endpoint.Scheme)
	}
	endpoint.RawQuery = query.Encode()
	header := make(http.Header)
	if c.secret != "" {
		header.Set("Authorization", "Bearer "+c.secret)
	}
	conn, response, err := websocket.Dial(ctx, endpoint.String(), &websocket.DialOptions{HTTPClient: c.http, HTTPHeader: header})
	if err != nil {
		if response != nil {
			return fmt.Errorf("%s WebSocket handshake returned %s: %w", path, response.Status, err)
		}
		return fmt.Errorf("connect %s WebSocket: %w", path, err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxStreamMessageSize)
	for {
		if err := read(conn); err != nil {
			return err
		}
	}
}
