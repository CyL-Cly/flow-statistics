package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// MetaCache resolves MAC → IP / hostname with light caching (DHCP + ARP).
type MetaCache struct {
	leasesPath string
	arpPath    string

	mu       sync.Mutex
	byMAC    map[string]deviceMeta // MAC → meta
	byIP     map[string]string     // IP → MAC
	lastDHCP time.Time
	lastARP  time.Time
}

func newMetaCache(leases, arp string) *MetaCache {
	return &MetaCache{
		leasesPath: leases,
		arpPath:    arp,
		byMAC:      make(map[string]deviceMeta),
		byIP:       make(map[string]string),
	}
}

// ResolveByMAC returns best-effort IP/name for a station MAC.
func (m *MetaCache) ResolveByMAC(mac, ifaceHint string) deviceMeta {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	if now.Sub(m.lastDHCP) > 15*time.Second {
		m.reloadDHCP()
		m.lastDHCP = now
	}
	if now.Sub(m.lastARP) > 10*time.Second {
		m.reloadARP()
		m.lastARP = now
	}

	mac = strings.ToUpper(strings.TrimSpace(mac))
	meta, ok := m.byMAC[mac]
	if !ok {
		meta = deviceMeta{MAC: mac}
	}
	if ifaceHint != "" {
		meta.Iface = ifaceHint
	}
	if meta.Name == "" {
		if meta.IP != "" {
			meta.Name = meta.IP
		} else {
			meta.Name = mac
		}
	}
	m.byMAC[mac] = meta
	return meta
}

// dhcp.leases: <expiry> <mac> <ip> <hostname> <clientid>
func (m *MetaCache) reloadDHCP() {
	f, err := os.Open(m.leasesPath)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 {
			continue
		}
		mac := strings.ToUpper(fields[1])
		ip := fields[2]
		name := fields[3]
		if name == "*" {
			name = ""
		}
		cur := m.byMAC[mac]
		cur.MAC = mac
		cur.IP = ip
		if name != "" {
			cur.Name = name
		}
		m.byMAC[mac] = cur
		m.byIP[ip] = mac
	}
}

// /proc/net/arp: IP address HW type Flags HW address Mask Device
func (m *MetaCache) reloadARP() {
	f, err := os.Open(m.arpPath)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		line := sc.Text()
		if first {
			first = false
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		ip := fields[0]
		mac := strings.ToUpper(fields[3])
		if mac == "00:00:00:00:00:00" || !isMAC(mac) {
			continue
		}
		cur := m.byMAC[mac]
		cur.MAC = mac
		// Prefer DHCP IP if already known; else take ARP.
		if cur.IP == "" {
			cur.IP = ip
		}
		if cur.Iface == "" {
			cur.Iface = fields[5]
		}
		m.byMAC[mac] = cur
		m.byIP[ip] = mac
	}
}

func isMAC(s string) bool {
	if len(s) != 17 {
		return false
	}
	for i, c := range s {
		if i%3 == 2 {
			if c != ':' {
				return false
			}
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// ensureMAC fills a synthetic MAC when none is known (should be rare).
func ensureMAC(mac, ip string) string {
	if mac != "" {
		return strings.ToUpper(mac)
	}
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		var b [4]int
		for i := 0; i < 4; i++ {
			fmt.Sscanf(parts[i], "%d", &b[i])
		}
		return fmt.Sprintf("02:00:%02X:%02X:%02X:%02X", b[0], b[1], b[2], b[3])
	}
	return "02:00:00:00:00:00"
}
