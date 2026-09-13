package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPlaceholderToken = "change-me-traffic-token"
	minSampleInterval       = 5 * time.Second
	defaultHTTPTimeout      = 8 * time.Second
	defaultQueueMax         = 120
)

var (
	errTokenRequired = errors.New("TRAFFIC_TOKEN is required (refusing empty or placeholder token)")
	errServerHTTPS   = errors.New("TRAFFIC_SERVER must be https:// (http is only allowed for loopback)")
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

func defaultConfigFromEnv() Config {
	return Config{
		ServerURL:   env("TRAFFIC_SERVER", "http://127.0.0.1:8080/api/v1/traffic/report"),
		Token:       env("TRAFFIC_TOKEN", ""),
		RouterID:    env("TRAFFIC_ROUTER_ID", "main-router-01"),
		Interval:    envDuration("TRAFFIC_INTERVAL", 30*time.Second),
		DHCPLeases:  env("TRAFFIC_DHCP_LEASES", "/tmp/dhcp.leases"),
		ARPPath:     env("TRAFFIC_ARP", "/proc/net/arp"),
		WiFiIfaces:  splitCSV(env("TRAFFIC_WIFI_IFACES", "")),
		QueueMax:    envInt("TRAFFIC_QUEUE_MAX", defaultQueueMax),
		HTTPTimeout: envDuration("TRAFFIC_HTTP_TIMEOUT", defaultHTTPTimeout),
		Verbose:     envBool("TRAFFIC_VERBOSE", false),
	}
}

func loadConfig() (Config, error) {
	cfg := defaultConfigFromEnv()

	var ifacesFlag string
	flag.StringVar(&cfg.ServerURL, "server", cfg.ServerURL, "report URL")
	// -token is accepted for old init scripts but is visible in `ps`; prefer TRAFFIC_TOKEN.
	flag.StringVar(&cfg.Token, "token", cfg.Token, "X-Device-Token (prefer TRAFFIC_TOKEN env)")
	flag.StringVar(&cfg.RouterID, "router-id", cfg.RouterID, "router id")
	flag.DurationVar(&cfg.Interval, "interval", cfg.Interval, "sample interval")
	flag.StringVar(&cfg.DHCPLeases, "leases", cfg.DHCPLeases, "dhcp.leases path")
	flag.StringVar(&cfg.ARPPath, "arp", cfg.ARPPath, "arp table path")
	flag.StringVar(&ifacesFlag, "wifi-ifaces", "", "comma-separated AP ifaces (empty=auto)")
	flag.IntVar(&cfg.QueueMax, "queue-max", cfg.QueueMax, "max offline FIFO reports")
	flag.DurationVar(&cfg.HTTPTimeout, "http-timeout", cfg.HTTPTimeout, "HTTP client timeout")
	flag.BoolVar(&cfg.Verbose, "verbose", cfg.Verbose, "log each sample summary (default false)")
	flag.Parse()

	if ifacesFlag != "" {
		cfg.WiFiIfaces = splitCSV(ifacesFlag)
	}
	if err := cfg.normalize(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) normalize() error {
	c.ServerURL = strings.TrimSpace(c.ServerURL)
	c.Token = strings.TrimSpace(c.Token)
	c.RouterID = strings.TrimSpace(c.RouterID)

	if c.Interval < minSampleInterval {
		c.Interval = minSampleInterval
	}
	if c.HTTPTimeout <= 0 {
		c.HTTPTimeout = defaultHTTPTimeout
	}
	if c.QueueMax <= 0 {
		c.QueueMax = defaultQueueMax
	}
	if c.Token == "" || c.Token == defaultPlaceholderToken {
		return errTokenRequired
	}
	if c.RouterID == "" {
		return fmt.Errorf("TRAFFIC_ROUTER_ID is required")
	}
	if err := validateServerURL(c.ServerURL); err != nil {
		return err
	}
	return nil
}

func validateServerURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("TRAFFIC_SERVER is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("invalid TRAFFIC_SERVER %q", raw)
	}
	if u.Scheme != "https" && !isLoopbackURL(u) {
		return errServerHTTPS
	}
	return nil
}

func isLoopbackURL(u *url.URL) bool {
	host := u.Hostname()
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
