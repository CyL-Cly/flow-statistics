package main

import (
	"log"
	"time"
)

const ifaceRefresh = 5 * time.Minute

// stationReader collects associated Wi-Fi stations. Tests stub this to avoid `iw`.
type stationReader func(ifaces []string) ([]wifiStation, error)

// Collector samples Wi-Fi station cumulative counters and emits per-interval deltas.
//
// Data source: `iw dev <ap> station dump` (driver / mac80211 counters).
// Scope: Wi-Fi associated devices only (wired clients are not counted).
type Collector struct {
	cfg  Config
	meta *MetaCache

	readStations stationReader

	prev         map[string]wifiStation
	primed       bool
	lastSampleAt time.Time

	cachedIfaces []string
	ifacesAt     time.Time
}

func newCollector(cfg Config) *Collector {
	return &Collector{
		cfg:          cfg,
		meta:         newMetaCache(cfg.DHCPLeases, cfg.ARPPath),
		readStations: readWiFiStations,
		prev:         make(map[string]wifiStation),
	}
}

func (c *Collector) apIfaces() []string {
	if len(c.cfg.WiFiIfaces) > 0 {
		return c.cfg.WiFiIfaces
	}
	if len(c.cachedIfaces) > 0 && time.Since(c.ifacesAt) < ifaceRefresh {
		return c.cachedIfaces
	}
	c.cachedIfaces = discoverAPIfaces()
	c.ifacesAt = time.Now()
	if c.cfg.Verbose {
		log.Printf("collector: AP ifaces=%v", c.cachedIfaces)
	}
	return c.cachedIfaces
}

// Sample returns a report with per-device byte deltas over the last interval.
// First successful call establishes baseline and returns nil.
// Any iw error keeps the previous snapshot and returns an empty heartbeat
// payload (no devices) so the server still sees the router online.
func (c *Collector) Sample() *ReportPayload {
	now := time.Now()
	ifaces := c.apIfaces()

	read := c.readStations
	if read == nil {
		read = readWiFiStations
	}
	stations, err := read(ifaces)
	if err != nil {
		log.Printf("wifi station read error: %v", err)
		c.ifacesAt = time.Time{} // rediscover next tick
		// Emit an empty heartbeat so the router is not marked offline while
		// iw keeps failing; deltas stay correct because the baseline is kept.
		elapsed := now.Sub(c.lastSampleAt)
		if !c.primed || elapsed <= 0 {
			elapsed = c.cfg.Interval
		}
		return c.makePayload([]DeviceReport{}, elapsed)
	}

	cur := dedupeStations(stations)

	if !c.primed {
		c.prev = cur
		c.primed = true
		c.lastSampleAt = now
		if c.cfg.Verbose {
			log.Printf("collector: baseline stations=%d", len(cur))
		}
		return nil
	}

	devices := c.deviceDeltas(cur)
	c.prev = cur

	elapsed := now.Sub(c.lastSampleAt)
	c.lastSampleAt = now
	return c.makePayload(devices, elapsed)
}

func dedupeStations(stations []wifiStation) map[string]wifiStation {
	cur := make(map[string]wifiStation, len(stations))
	for _, s := range stations {
		mac := stringsToUpperMAC(s.MAC)
		if mac == "" {
			continue
		}
		s.MAC = mac
		if prev, ok := cur[mac]; ok {
			if s.RxBytes+s.TxBytes >= prev.RxBytes+prev.TxBytes {
				cur[mac] = s
			}
			continue
		}
		cur[mac] = s
	}
	return cur
}

func (c *Collector) deviceDeltas(cur map[string]wifiStation) []DeviceReport {
	devices := make([]DeviceReport, 0, len(cur))
	for mac, s := range cur {
		drx, dtx := int64(0), int64(0)
		if p, ok := c.prev[mac]; ok {
			if p.Iface != "" && s.Iface != "" && p.Iface != s.Iface &&
				(s.RxBytes < p.RxBytes || s.TxBytes < p.TxBytes) {
				// Roamed to another AP whose counters start lower: new baseline, no burst.
			} else {
				drx = entryDelta(s.RxBytes, p.RxBytes)
				dtx = entryDelta(s.TxBytes, p.TxBytes)
			}
		}

		if drx == 0 && dtx == 0 {
			continue
		}

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
	return devices
}

func (c *Collector) makePayload(devices []DeviceReport, elapsed time.Duration) *ReportPayload {
	sec := int(elapsed.Round(time.Second) / time.Second)
	if sec < 1 {
		sec = 1
	}

	if c.cfg.Verbose && len(devices) > 0 {
		var totalDelta int64
		for _, d := range devices {
			totalDelta += d.RxBytes + d.TxBytes
		}
		log.Printf("sample devices=%d stations=%d delta_bytes=%d (avg %.2f KB/s over %ds)",
			len(devices), len(c.prev), totalDelta, float64(totalDelta)/float64(sec)/1024, sec)
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
		return ""
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
