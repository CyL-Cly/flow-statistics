package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const iwCommandTimeout = 5 * time.Second

// wifiStation is one associated client with driver-level cumulative counters.
// Fields are already mapped to the *client* perspective used by our API:
//
//	iw "tx bytes" (AP → station) → RxBytes (client download)
//	iw "rx bytes" (station → AP) → TxBytes (client upload)
type wifiStation struct {
	MAC     string
	Iface   string
	RxBytes int64 // client download
	TxBytes int64 // client upload
}

func discoverAPIfaces() []string {
	b, err := runIW(iwCommandTimeout, "dev")
	if err == nil {
		if ifaces := parseIWDev(b); len(ifaces) > 0 {
			return ifaces
		}
	}
	return discoverAPIfacesFallback()
}

func discoverAPIfacesFallback() []string {
	candidates := []string{"phy0-ap0", "phy1-ap0", "phy0-ap1", "phy1-ap1", "wlan0", "wlan1"}
	var out []string
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join("/sys/class/net", c)); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// parseIWDev returns netdev names whose type is AP (including AP-VLAN) from `iw dev`.
func parseIWDev(b []byte) []string {
	var (
		out  []string
		name string
	)
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "Interface ") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "Interface "))
			continue
		}
		if name == "" || !strings.HasPrefix(line, "type ") {
			continue
		}
		typ := strings.TrimSpace(strings.TrimPrefix(line, "type "))
		if isAPType(typ) && validNetIface(name) {
			out = append(out, name)
		}
		name = ""
	}
	return out
}

func isAPType(typ string) bool {
	u := strings.ToUpper(typ)
	return u == "AP" || strings.HasPrefix(u, "AP/") || strings.HasPrefix(u, "AP-")
}

func validNetIface(s string) bool {
	if s == "" || len(s) > 15 || s[0] == '-' || s[0] == '.' {
		return false
	}
	for _, c := range s {
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '.' || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func runIW(timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "iw", args...)
	return cmd.Output()
}

// readWiFiStations collects per-station cumulative rx/tx via `iw station dump`.
// Any iface error is returned so the collector can keep the previous snapshot.
func readWiFiStations(ifaces []string) ([]wifiStation, error) {
	if len(ifaces) == 0 {
		return nil, fmt.Errorf("no AP ifaces")
	}
	out := make([]wifiStation, 0, 32)
	var firstErr error
	for _, iface := range ifaces {
		if !validNetIface(iface) {
			if firstErr == nil {
				firstErr = fmt.Errorf("invalid iface %q", iface)
			}
			continue
		}
		sts, err := readIWStations(iface)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out = append(out, sts...)
	}
	return out, firstErr
}

func readIWStations(iface string) ([]wifiStation, error) {
	b, err := runIW(iwCommandTimeout, "dev", iface, "station", "dump")
	if err != nil {
		return nil, err
	}
	return parseIWStationDump(b, iface), nil
}

// parseIWStationDump parses `iw dev <ap> station dump`.
//
//	Station aa:bb:cc:dd:ee:ff (on phy0-ap0)
//		rx bytes:	123456
//		tx bytes:	654321
func parseIWStationDump(b []byte, iface string) []wifiStation {
	var out []wifiStation
	var cur *wifiStation

	flush := func() {
		if cur == nil {
			return
		}
		out = append(out, *cur)
		cur = nil
	}

	sc := bufio.NewScanner(bytes.NewReader(b))
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "Station ") {
			flush()
			fields := strings.Fields(line)
			mac := ""
			if len(fields) >= 2 {
				mac = stringsToUpperMAC(fields[1])
			}
			if mac == "" {
				cur = nil
				continue
			}
			cur = &wifiStation{
				MAC:   mac,
				Iface: iface,
			}
			continue
		}
		if cur == nil {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "rx bytes:"):
			if n, ok := parseIWInt(line); ok {
				cur.TxBytes = n
			}
		case strings.HasPrefix(lower, "tx bytes:"):
			if n, ok := parseIWInt(line); ok {
				cur.RxBytes = n
			}
		}
	}
	flush()
	return out
}

func parseIWInt(line string) (int64, bool) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return 0, false
	}
	s := strings.TrimSpace(line[i+1:])
	if sp := strings.IndexByte(s, ' '); sp > 0 {
		s = s[:sp]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
