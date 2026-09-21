package config

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"strings"
	"testing"
	"time"
)

func environment(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) { value, ok := values[key]; return value, ok }
}

func TestDefaultsAndPrecedence(t *testing.T) {
	c, _, err := Parse(nil, environment(nil), io.Discard)
	if err != nil || c != Defaults() {
		t.Fatalf("defaults: %+v %v", c, err)
	}
	c, _, err = Parse([]string{"--connection-interval=2s", "--mihomo-secret=cli", "--output-prometheus=false", "--output-telegraf"}, environment(map[string]string{
		"CONNECTION_INTERVAL": "3s", "MIHOMO_SECRET": "", "OUTPUT_PROMETHEUS": "true",
	}), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.Mihomo.ConnectionInterval != 3*time.Second || c.Mihomo.Secret != "" || !c.Prometheus.Enabled || !c.Telegraf.Enabled {
		t.Fatalf("incorrect precedence: %+v", c)
	}
}

func TestInvalidInputs(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		env  map[string]string
	}{
		{"unknown flag", []string{"--unknown"}, nil},
		{"positional", []string{"unexpected"}, nil},
		{"bad duration", nil, map[string]string{"HTTP_TIMEOUT": "oops"}},
		{"bad bool", nil, map[string]string{"OUTPUT_TELEGRAF": "maybe"}},
		{"sub millisecond", []string{"--connection-interval=1us"}, nil},
		{"fractional millisecond", []string{"--connection-interval=1500us"}, nil},
		{"negative duration", []string{"--reconnect-interval=-1s"}, nil},
		{"no outputs", []string{"--output-prometheus=false"}, nil},
		{"bad URL", []string{"--mihomo-url=ftp://localhost"}, nil},
		{"bad listen", []string{"--prometheus-listen=localhost"}, nil},
		{"bad port", []string{"--prometheus-listen=:99999"}, nil},
		{"bad level", []string{"--log-level=trace"}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := Parse(tt.args, environment(tt.env), io.Discard); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestHelpAndVersionIgnoreEnvironment(t *testing.T) {
	env := environment(map[string]string{"MIHOMO_SECRET": "hidden-secret", "HTTP_TIMEOUT": "invalid"})
	var output bytes.Buffer
	if _, _, err := Parse([]string{"--help"}, env, &output); !errors.Is(err, flag.ErrHelp) {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "hidden-secret") {
		t.Fatal("help exposed secret")
	}
	if _, version, err := Parse([]string{"--version"}, env, io.Discard); err != nil || !version {
		t.Fatalf("version=%v err=%v", version, err)
	}
}
