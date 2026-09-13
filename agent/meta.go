package main

import (
	"bufio"
	"log"
	"net"
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
	byMAC    map[string]deviceMeta
	lastLoad time.Time
}

func newMetaCache(leases, arp string) *MetaCache {
	return &MetaCache{
		leasesPath: leases,
		arpPath:    arp,
		byMAC:      make(map[string]deviceMeta),
	}
}

// ResolveByMAC returns best-effort IP/name for a station MAC.
func (m *MetaCache) ResolveByMAC(mac, ifaceHint string) deviceMeta {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	if now.Sub(m.lastLoad) > 15*time.Second {
		m.reload()
		m.lastLoad = now
	}

	mac = stringsToUpperMAC(mac)
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
	return meta
}

func (m *MetaCache) reload() {
	next := make(map[string]deviceMeta)
	dhcpOK := m.loadDHCP(next)
	arpOK := m.loadARP(next)
	if !dhcpOK && !arpOK && len(m.byMAC) > 0 {
		return
	}
	m.byMAC = next
}

// dhcp.leases: <expiry> <mac> <ip> <hostname> <clientid>
func (m *MetaCache) loadDHCP(dst map[string]deviceMeta) bool {
	f, err := os.Open(m.leasesPath)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 {
			continue
		}
		mac := stringsToUpperMAC(fields[1])
		if mac == "" {
			continue
		}
		ip := fields[2]
		if net.ParseIP(ip) == nil {
			continue
		}
		name := fields[3]
		if name == "*" {
			name = ""
		}
		cur := dst[mac]
		cur.MAC = mac
		cur.IP = ip
		if name != "" {
			cur.Name = name
		}
		dst[mac] = cur
	}
	if err := sc.Err(); err != nil {
		log.Printf("dhcp.leases scan: %v", err)
	}
	return true
}

// /proc/net/arp: IP address HW type Flags HW address Mask Device
func (m *MetaCache) loadARP(dst map[string]deviceMeta) bool {
	f, err := os.Open(m.arpPath)
	if err != nil {
		return false
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
		if net.ParseIP(ip) == nil {
			continue
		}
		mac := stringsToUpperMAC(fields[3])
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}
		cur := dst[mac]
		cur.MAC = mac
		if cur.IP == "" {
			cur.IP = ip
		}
		if cur.Iface == "" {
			cur.Iface = fields[5]
		}
		dst[mac] = cur
	}
	if err := sc.Err(); err != nil {
		log.Printf("arp scan: %v", err)
	}
	return true
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
