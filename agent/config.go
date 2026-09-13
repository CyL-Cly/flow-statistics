package main

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime settings for the OpenWrt agent.
type Config struct {
	ServerURL   string
	Token       string
	RouterID    string
	Interval    time.Duration
	DHCPLeases  string
	ARPPath     string
	WiFiIfaces  []string // empty → auto-discover AP ifaces
	QueueMax    int
	HTTPTimeout time.Duration
	// Verbose enables per-sample summary logs (noisy on OpenWrt logd).
	// Errors and startup lines are always logged.
	Verbose bool
}

func loadConfig() Config {
	cfg := Config{
		ServerURL:   env("TRAFFIC_SERVER", "http://127.0.0.1:8080/api/v1/traffic/report"),
		Token:       env("TRAFFIC_TOKEN", "change-me-traffic-token"),
		RouterID:    env("TRAFFIC_ROUTER_ID", "main-router-01"),
		// Accurate cumulative counters: longer interval is fine and cheaper.
		Interval:    envDuration("TRAFFIC_INTERVAL", 30*time.Second),
		DHCPLeases:  env("TRAFFIC_DHCP_LEASES", "/tmp/dhcp.leases"),
		ARPPath:     env("TRAFFIC_ARP", "/proc/net/arp"),
		WiFiIfaces:  splitCSV(env("TRAFFIC_WIFI_IFACES", "")),
		QueueMax:    envInt("TRAFFIC_QUEUE_MAX", 120), // ~1h at 30s
		HTTPTimeout: envDuration("TRAFFIC_HTTP_TIMEOUT", 8*time.Second),
		// Default quiet: sample spam filled OpenWrt's 128KB logd buffer.
		Verbose: envBool("TRAFFIC_VERBOSE", false),
	}

	var ifacesFlag string
	flag.StringVar(&cfg.ServerURL, "server", cfg.ServerURL, "report URL")
	flag.StringVar(&cfg.Token, "token", cfg.Token, "X-Device-Token")
	flag.StringVar(&cfg.RouterID, "router-id", cfg.RouterID, "router id")
	flag.DurationVar(&cfg.Interval, "interval", cfg.Interval, "sample interval")
	flag.StringVar(&cfg.DHCPLeases, "leases", cfg.DHCPLeases, "dhcp.leases path")
	flag.StringVar(&cfg.ARPPath, "arp", cfg.ARPPath, "arp table path")
	flag.StringVar(&ifacesFlag, "wifi-ifaces", "", "comma-separated AP ifaces (empty=auto)")
	flag.IntVar(&cfg.QueueMax, "queue-max", cfg.QueueMax, "max offline FIFO reports")
	flag.BoolVar(&cfg.Verbose, "verbose", cfg.Verbose, "log each sample summary (default false)")
	flag.Parse()

	if ifacesFlag != "" {
		cfg.WiFiIfaces = splitCSV(ifacesFlag)
	}
	cfg.ServerURL = strings.TrimSpace(cfg.ServerURL)
	return cfg
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envDuration(k string, def time.Duration) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	return def
}

func envBool(k string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "on", "y":
		return true
	case "0", "false", "no", "off", "n":
		return false
	default:
		return def
	}
}
