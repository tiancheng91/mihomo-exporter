package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestInformationalCommandsAndUsageErrors(t *testing.T) {
	for _, tt := range []struct {
		arg  string
		code int
	}{{"--help", 0}, {"--version", 0}, {"--unknown", 2}} {
		t.Run(tt.arg, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := run(context.Background(), []string{tt.arg}, func(string) (string, bool) { return "", false }, &out, &errOut)
			if code != tt.code {
				t.Fatalf("code=%d stderr=%s", code, &errOut)
			}
			if tt.arg == "--version" && !strings.Contains(out.String(), version) {
				t.Fatal("version missing")
			}
		})
	}
}
