package mihomo

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStreamTraffic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing auth")
		}
		fmt.Fprint(w, "{\"up\":1,\"downTotal\":2}\n{\"up\":3,\"downTotal\":4}\n")
	}))
	defer server.Close()
	c := NewClient(server.URL, "secret", time.Second)
	var got []Traffic
	err := c.StreamTraffic(context.Background(), func(v Traffic) { got = append(got, v) })
	if err == nil || len(got) != 2 || got[1].Up != 3 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestStreamConnectionsInterval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connections" || r.URL.Query().Get("interval") != "1500" {
			t.Errorf("url=%s", r.URL)
		}
		fmt.Fprint(w, "{\"connections\":[{\"id\":\"x\",\"metadata\":{\"sourceIP\":\"c\"}}]}")
	}))
	defer server.Close()
	c := NewClient(server.URL, "", time.Second)
	var got ConnectionsSnapshot
	_ = c.StreamConnections(context.Background(), 1500*time.Millisecond, func(v ConnectionsSnapshot) { got = v })
	if len(got.Connections) != 1 || got.Connections[0].ID != "x" {
		t.Fatalf("got=%+v", got)
	}
}
