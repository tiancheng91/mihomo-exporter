// Package config parses and validates runtime configuration.
package config

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Mihomo            Mihomo
	Prometheus        Prometheus
	Telegraf          Telegraf
	HTTP              HTTP
	EnableClientProxy bool
	LogLevel          string
}

type Mihomo struct {
	URL                string
	Secret             string
	ConnectionInterval time.Duration
	ReconnectInterval  time.Duration
}

type Prometheus struct {
	Enabled bool
	Listen  string
}

type Telegraf struct {
	Enabled       bool
	URL           string
	FlushInterval time.Duration
}

type HTTP struct {
	Timeout time.Duration
}

// Defaults returns a fresh configuration with no dependency on process state.
func Defaults() Config {
	return Config{
		Mihomo:     Mihomo{URL: "http://127.0.0.1:9090", ConnectionInterval: time.Second, ReconnectInterval: 2 * time.Second},
		Prometheus: Prometheus{Enabled: true, Listen: ":9091"},
		Telegraf:   Telegraf{URL: "http://127.0.0.1:8186/mihomo", FlushInterval: 10 * time.Second},
		HTTP:       HTTP{Timeout: 5 * time.Second},
		LogLevel:   "info",
	}
}

// Parse uses environment > explicit flags > defaults, preserving the original
// environment-first contract. lookupEnv and output are injected for testing.
// Help returns flag.ErrHelp; version requests bypass environment and validation.
func Parse(args []string, lookupEnv func(string) (string, bool), output io.Writer) (Config, bool, error) {
	c := Defaults()
	fs := flag.NewFlagSet("mihomo-exporter", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.Usage = func() {
		fmt.Fprintln(output, "Usage: mihomo-exporter [flags]")
		fmt.Fprintln(output, "Precedence: environment > flags > defaults. Environment names are shown below.")
		fs.PrintDefaults()
	}
	fs.StringVar(&c.Mihomo.URL, "mihomo-url", c.Mihomo.URL, "Mihomo controller URL (MIHOMO_URL)")
	fs.StringVar(&c.Mihomo.Secret, "mihomo-secret", "", "Bearer secret; prefer environment to avoid shell history (MIHOMO_SECRET)")
	fs.DurationVar(&c.Mihomo.ConnectionInterval, "connection-interval", c.Mihomo.ConnectionInterval, "Snapshot interval (CONNECTION_INTERVAL)")
	fs.DurationVar(&c.Mihomo.ReconnectInterval, "reconnect-interval", c.Mihomo.ReconnectInterval, "Reconnect delay (RECONNECT_INTERVAL)")
	fs.BoolVar(&c.Prometheus.Enabled, "output-prometheus", c.Prometheus.Enabled, "Enable Prometheus (OUTPUT_PROMETHEUS)")
	fs.StringVar(&c.Prometheus.Listen, "prometheus-listen", c.Prometheus.Listen, "Listen address (PROMETHEUS_LISTEN)")
	fs.BoolVar(&c.Telegraf.Enabled, "output-telegraf", c.Telegraf.Enabled, "Enable Telegraf (OUTPUT_TELEGRAF)")
	fs.StringVar(&c.Telegraf.URL, "telegraf-url", c.Telegraf.URL, "Push URL (TELEGRAF_URL)")
	fs.DurationVar(&c.Telegraf.FlushInterval, "flush-interval", c.Telegraf.FlushInterval, "Push interval (FLUSH_INTERVAL)")
	fs.DurationVar(&c.HTTP.Timeout, "http-timeout", c.HTTP.Timeout, "HTTP timeout (HTTP_TIMEOUT)")
	fs.BoolVar(&c.EnableClientProxy, "enable-client-proxy", false, "Enable combined dimension (ENABLE_CLIENT_PROXY)")
	fs.StringVar(&c.LogLevel, "log-level", c.LogLevel, "Log level (LOG_LEVEL)")
	showVersion := fs.Bool("version", false, "Print version and exit")
	if err := fs.Parse(args); err != nil {
		return Config{}, false, err
	}
	if fs.NArg() != 0 {
		return Config{}, false, fmt.Errorf("unexpected positional arguments")
	}
	if *showVersion {
		return c, true, nil
	}
	// The registered flag setters provide one parser for both input sources.
	var envErr error
	fs.VisitAll(func(f *flag.Flag) {
		if f.Name == "version" || envErr != nil {
			return
		}
		key := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		if value, ok := lookupEnv(key); ok {
			if err := f.Value.Set(value); err != nil {
				envErr = fmt.Errorf("%s: invalid value for %s", key, f.Name)
			}
		}
	})
	if envErr != nil {
		return Config{}, false, envErr
	}
	c.LogLevel = strings.ToLower(c.LogLevel)
	return c, false, c.Validate()
}

// Validate is also used by callers that construct Config directly.
func (c Config) Validate() error {
	if !c.Prometheus.Enabled && !c.Telegraf.Enabled {
		return fmt.Errorf("at least one output must be enabled")
	}
	if err := validHTTPURL("MIHOMO_URL", c.Mihomo.URL); err != nil {
		return err
	}
	if c.Telegraf.Enabled {
		if err := validHTTPURL("TELEGRAF_URL", c.Telegraf.URL); err != nil {
			return err
		}
	}
	if c.Mihomo.ConnectionInterval < time.Millisecond || c.Mihomo.ConnectionInterval%time.Millisecond != 0 {
		return fmt.Errorf("CONNECTION_INTERVAL must be a positive whole number of milliseconds")
	}
	for _, item := range []struct {
		name  string
		value time.Duration
	}{
		{"FLUSH_INTERVAL", c.Telegraf.FlushInterval}, {"HTTP_TIMEOUT", c.HTTP.Timeout}, {"RECONNECT_INTERVAL", c.Mihomo.ReconnectInterval},
	} {
		if item.value <= 0 {
			return fmt.Errorf("%s must be greater than zero", item.name)
		}
	}
	if c.Prometheus.Enabled {
		_, port, err := net.SplitHostPort(c.Prometheus.Listen)
		if err != nil {
			return fmt.Errorf("PROMETHEUS_LISTEN must be host:port")
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 0 || n > 65535 {
			return fmt.Errorf("PROMETHEUS_LISTEN has an invalid port")
		}
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error")
	}
	return nil
}

func validHTTPURL(key, value string) error {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return fmt.Errorf("%s must be a valid http(s) URL", key)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%s must not contain credentials, query parameters, or fragments", key)
	}
	return nil
}
