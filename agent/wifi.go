package main

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

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

// discoverAPIfaces finds wireless AP netdevs (verified on this router: phy0-ap0, phy1-ap0).
func discoverAPIfaces() []string {
	candidates := []string{"phy0-ap0", "phy1-ap0", "phy0-ap1", "phy1-ap1", "wlan0", "wlan1"}
	var out []string
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join("/sys/class/net", c)); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// readWiFiStations collects per-station cumulative rx/tx via `iw station dump`.
func readWiFiStations(ifaces []string) ([]wifiStation, error) {
	if len(ifaces) == 0 {
		ifaces = discoverAPIfaces()
	}
	out := make([]wifiStation, 0, 32)
	var lastErr error
	for _, iface := range ifaces {
		sts, err := readIWStations(iface)
		if err != nil {
			lastErr = err
			continue
		}
		out = append(out, sts...)
	}
	if len(out) == 0 {
		return out, lastErr
	}
	return out, nil
}

func readIWStations(iface string) ([]wifiStation, error) {
	b, err := exec.Command("iw", "dev", iface, "station", "dump").Output()
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
			if len(fields) < 2 || !isMAC(fields[1]) {
				cur = nil
				continue
			}
			cur = &wifiStation{
				MAC:   strings.ToUpper(fields[1]),
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
			// AP received from station → client upload
			cur.TxBytes = parseIWInt(line)
		case strings.HasPrefix(lower, "tx bytes:"):
			// AP sent to station → client download
			cur.RxBytes = parseIWInt(line)
		}
	}
	flush()
	return out
}

func parseIWInt(line string) int64 {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return 0
	}
	s := strings.TrimSpace(line[i+1:])
	if sp := strings.IndexByte(s, ' '); sp > 0 {
		s = s[:sp]
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	if n < 0 {
		return 0
	}
	return n
}
