package main

import (
	"log"
	"time"
)

// Collector samples Wi-Fi station cumulative counters and emits per-interval deltas.
//
// Data source: `iw dev <ap> station dump` (driver / mac80211 counters).
// These are real L2 cumulative bytes for associated clients — far more accurate
// for daily totals than PPE FOE / nf_conntrack session snapshots.
//
// Scope: Wi-Fi associated devices only (wired clients are not counted).
type Collector struct {
	cfg  Config
	meta *MetaCache

	// previous absolute counters keyed by MAC
	prev map[string]wifiStation

	// first sample only establishes baseline
	primed bool
}

func newCollector(cfg Config) *Collector {
	ifaces := cfg.WiFiIfaces
	if len(ifaces) == 0 {
		ifaces = discoverAPIfaces()
	}
	if cfg.Verbose {
		log.Printf("collector: Wi-Fi station counters (accurate daily totals), ifaces=%v", ifaces)
	}
	return &Collector{
		cfg:  cfg,
		meta: newMetaCache(cfg.DHCPLeases, cfg.ARPPath),
		prev: make(map[string]wifiStation),
	}
}

// Sample returns a report with per-device byte deltas over the last interval.
// First call establishes baseline and returns nil.
func (c *Collector) Sample() *ReportPayload {
	ifaces := c.cfg.WiFiIfaces
	if len(ifaces) == 0 {
		ifaces = discoverAPIfaces()
	}

	stations, err := readWiFiStations(ifaces)
	if err != nil && len(stations) == 0 {
		log.Printf("wifi station read error: %v", err)
	}

	// Deduplicate by MAC (same client should not appear on two APs; if it does, prefer larger counters).
	cur := make(map[string]wifiStation, len(stations))
	for _, s := range stations {
		mac := stringsToUpperMAC(s.MAC)
		if mac == "" {
			continue
		}
		s.MAC = mac
		if prev, ok := cur[mac]; ok {
			// keep the entry with more total traffic (likely the active AP)
			if s.RxBytes+s.TxBytes >= prev.RxBytes+prev.TxBytes {
				cur[mac] = s
			}
			continue
		}
		cur[mac] = s
	}

	if !c.primed {
		c.prev = cur
		c.primed = true
		if c.cfg.Verbose {
			log.Printf("collector: baseline stations=%d", len(cur))
		}
		return nil
	}

	devices := make([]DeviceReport, 0, len(cur))
	var totalDelta int64

	for mac, s := range cur {
		var drx, dtx int64
		if p, ok := c.prev[mac]; ok {
			// Same association: positive growth only.
			// Counter went backwards → driver reset / reassoc: take current as new session bytes.
			drx = entryDelta(s.RxBytes, p.RxBytes)
			dtx = entryDelta(s.TxBytes, p.TxBytes)
		}
		// brand-new MAC this tick: baseline only (no delta) — avoids counting pre-agent history

		if drx == 0 && dtx == 0 {
			continue
		}
		totalDelta += drx + dtx

		// s.RxBytes / s.TxBytes already client download / upload (see wifi.go).
		meta := c.meta.ResolveByMAC(mac, s.Iface)
		devices = append(devices, DeviceReport{
			MAC:     mac,
			IP:      meta.IP,
			Name:    meta.Name,
			Iface:   firstNonEmpty(s.Iface, meta.Iface),
			RxBytes: drx,
			TxBytes: dtx,
		})
	}

	// Drop disappeared MACs from prev by replacing with current snapshot.
	c.prev = cur

	sec := int(c.cfg.Interval / time.Second)
	if sec <= 0 {
		sec = 30
	}

	if c.cfg.Verbose && len(devices) > 0 {
		log.Printf("sample devices=%d stations=%d delta_bytes=%d (avg %.2f KB/s over %ds)",
			len(devices), len(cur), totalDelta, float64(totalDelta)/float64(sec)/1024, sec)
	}

	return &ReportPayload{
		RouterID:    c.cfg.RouterID,
		Timestamp:   time.Now().Unix(),
		IntervalSec: sec,
		Devices:     devices,
	}
}

// entryDelta returns growth of a single cumulative counter.
// cur < prev → counter reset: count cur as new session bytes.
func entryDelta(cur, prev int64) int64 {
	if cur < prev {
		return cur
	}
	return cur - prev
}

func stringsToUpperMAC(mac string) string {
	if !isMAC(mac) {
		// still normalize if length ok
		if len(mac) != 17 {
			return ""
		}
	}
	b := make([]byte, len(mac))
	for i := 0; i < len(mac); i++ {
		c := mac[i]
		if c >= 'a' && c <= 'f' {
			c -= 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
