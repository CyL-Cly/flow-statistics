package main

import (
	"net/url"
	"testing"
	"time"
)

func TestConfigNormalizeClamps(t *testing.T) {
	cfg := Config{
		ServerURL:   "http://127.0.0.1:8080/api/v1/traffic/report",
		Token:       "secret-token",
		RouterID:    "r1",
		Interval:    time.Second,
		HTTPTimeout: 0,
		QueueMax:    0,
	}
	if err := cfg.normalize(); err != nil {
		t.Fatal(err)
	}
	if cfg.Interval != minSampleInterval {
		t.Fatalf("interval=%s", cfg.Interval)
	}
	if cfg.HTTPTimeout != defaultHTTPTimeout {
		t.Fatalf("timeout=%s", cfg.HTTPTimeout)
	}
	if cfg.QueueMax != defaultQueueMax {
		t.Fatalf("queue=%d", cfg.QueueMax)
	}
}

func TestConfigRejectsPlaceholderToken(t *testing.T) {
	cfg := Config{
		ServerURL: "http://127.0.0.1:8080/x",
		Token:     defaultPlaceholderToken,
		RouterID:  "r1",
		Interval:  30 * time.Second,
	}
	if err := cfg.normalize(); err != errTokenRequired {
		t.Fatalf("err=%v", err)
	}
	cfg.Token = ""
	if err := cfg.normalize(); err != errTokenRequired {
		t.Fatalf("empty token err=%v", err)
	}
}

func TestValidateServerURL(t *testing.T) {
	cases := []struct {
		url string
		ok  bool
	}{
		{"http://127.0.0.1:8080/api", true},
		{"http://localhost/api", true},
		{"https://dagongren.tech/api/v1/traffic/report", true},
		{"http://example.com/api", false},
		{"ftp://127.0.0.1/x", false},
		{"", false},
		{"not a url", false},
	}
	for _, tc := range cases {
		err := validateServerURL(tc.url)
		if tc.ok && err != nil {
			t.Errorf("%s: %v", tc.url, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("%s: expected error", tc.url)
		}
	}
}

func TestIsLoopbackURL(t *testing.T) {
	u, _ := url.Parse("http://[::1]/x")
	if !isLoopbackURL(u) {
		t.Fatal("::1")
	}
}
