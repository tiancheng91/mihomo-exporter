package mihomo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestStreamTraffic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/traffic" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing auth")
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		for _, traffic := range []Traffic{{Up: 1, DownTotal: 2}, {Up: 3, DownTotal: 4}} {
			if err := wsjson.Write(r.Context(), conn, traffic); err != nil {
				t.Error(err)
				return
			}
		}
		_ = conn.Close(websocket.StatusNormalClosure, "done")
	}))
	defer server.Close()
	c := NewClient(server.URL, "secret", time.Second)
	var got []Traffic
	err := c.StreamTraffic(context.Background(), func(v Traffic) { got = append(got, v) })
	if websocket.CloseStatus(err) != websocket.StatusNormalClosure || len(got) != 2 || got[1].Up != 3 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestStreamConnectionsInterval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connections" || r.URL.Query().Get("interval") != "1500" {
			t.Errorf("url=%s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing auth")
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		for _, id := range []string{"x", "y"} {
			if err := wsjson.Write(r.Context(), conn, ConnectionsSnapshot{Connections: []Connection{{ID: id}}}); err != nil {
				t.Error(err)
				return
			}
		}
		_ = conn.Close(websocket.StatusNormalClosure, "done")
	}))
	defer server.Close()
	c := NewClient(server.URL, "secret", time.Second)
	var got []ConnectionsSnapshot
	err := c.StreamConnections(context.Background(), 1500*time.Millisecond, func(v ConnectionsSnapshot) { got = append(got, v) })
	if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
		t.Fatalf("unexpected stream error: %v", err)
	}
	if len(got) != 2 || got[0].Connections[0].ID != "x" || got[1].Connections[0].ID != "y" {
		t.Fatalf("got=%+v", got)
	}
}

func TestStreamConnectionsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		_ = wsjson.Write(r.Context(), conn, ConnectionsSnapshot{})
		_, _, _ = conn.Read(context.Background())
	}))
	defer server.Close()
	c := NewClient(server.URL, "", time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	err := c.StreamConnections(ctx, time.Second, func(ConnectionsSnapshot) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestStreamConnectionsAllowsLargeSnapshot(t *testing.T) {
	largeClient := strings.Repeat("x", 128<<10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		if err := wsjson.Write(r.Context(), conn, ConnectionsSnapshot{Connections: []Connection{{ID: "large", Metadata: Metadata{SourceIP: largeClient}}}}); err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close(websocket.StatusNormalClosure, "done")
	}))
	defer server.Close()
	c := NewClient(server.URL, "", time.Second)
	var got string
	err := c.StreamConnections(context.Background(), time.Second, func(v ConnectionsSnapshot) { got = v.Connections[0].Metadata.SourceIP })
	if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
		t.Fatalf("unexpected stream error: %v", err)
	}
	if got != largeClient {
		t.Fatalf("large snapshot was truncated: got %d bytes", len(got))
	}
}
