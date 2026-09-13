package main

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func testCollector(t *testing.T, read stationReader) *Collector {
	t.Helper()
	dir := t.TempDir()
	c := newCollector(Config{
		Interval:   30 * time.Second,
		RouterID:   "t",
		WiFiIfaces: []string{"phy0-ap0"},
		DHCPLeases: filepath.Join(dir, "missing"),
		ARPPath:    filepath.Join(dir, "missing"),
	})
	c.readStations = read
	return c
}

func TestSampleBaselineThenDelta(t *testing.T) {
	n := 0
	c := testCollector(t, func([]string) ([]wifiStation, error) {
		n++
		rx := int64(1000)
		if n > 1 {
			rx = 1500
		}
		return []wifiStation{{MAC: "aa:bb:cc:dd:ee:01", Iface: "phy0-ap0", RxBytes: rx, TxBytes: 2000}}, nil
	})
	if p := c.Sample(); p != nil {
		t.Fatal("baseline should be nil")
	}
	c.lastSampleAt = time.Now().Add(-30 * time.Second)
	p := c.Sample()
	if p == nil || len(p.Devices) != 1 {
		t.Fatalf("payload=%+v", p)
	}
	if p.Devices[0].RxBytes != 500 || p.Devices[0].TxBytes != 0 {
		t.Fatalf("delta %+v", p.Devices[0])
	}
	if p.IntervalSec < 29 || p.IntervalSec > 31 {
		t.Fatalf("interval_sec=%d", p.IntervalSec)
	}
	if p.Devices[0].MAC != "AA:BB:CC:DD:EE:01" {
		t.Fatalf("mac=%s", p.Devices[0].MAC)
	}
}

func TestSampleFailedReadKeepsBaseline(t *testing.T) {
	n := 0
	c := testCollector(t, func([]string) ([]wifiStation, error) {
		n++
		if n == 1 {
			return []wifiStation{{MAC: "AA:BB:CC:DD:EE:01", Iface: "phy0-ap0", RxBytes: 1000, TxBytes: 2000}}, nil
		}
		if n == 2 {
			return nil, errors.New("iw failed")
		}
		return []wifiStation{{MAC: "AA:BB:CC:DD:EE:01", Iface: "phy0-ap0", RxBytes: 1400, TxBytes: 2100}}, nil
	})
	if c.Sample() != nil {
		t.Fatal("baseline")
	}
	if p := c.Sample(); p != nil {
		t.Fatalf("failed read should return nil, got %+v", p)
	}
	if !c.primed || len(c.prev) != 1 {
		t.Fatalf("primed=%v prev=%d", c.primed, len(c.prev))
	}
	c.lastSampleAt = time.Now().Add(-30 * time.Second)
	p := c.Sample()
	if p == nil || len(p.Devices) != 1 {
		t.Fatalf("payload=%+v", p)
	}
	if p.Devices[0].RxBytes != 400 || p.Devices[0].TxBytes != 100 {
		t.Fatalf("want 400/100 got %+v", p.Devices[0])
	}
}

func TestSamplePartialErrorKeepsBaseline(t *testing.T) {
	n := 0
	c := testCollector(t, func([]string) ([]wifiStation, error) {
		n++
		sts := []wifiStation{{MAC: "AA:BB:CC:DD:EE:01", Iface: "phy0-ap0", RxBytes: 1000, TxBytes: 2000}}
		if n == 1 {
			return sts, nil
		}
		if n == 2 {
			return []wifiStation{{MAC: "AA:BB:CC:DD:EE:01", Iface: "phy0-ap0", RxBytes: 1100, TxBytes: 2000}}, errors.New("phy1 timeout")
		}
		sts[0].RxBytes = 1400
		sts[0].TxBytes = 2100
		return sts, nil
	})
	if c.Sample() != nil {
		t.Fatal("baseline")
	}
	if p := c.Sample(); p != nil {
		t.Fatalf("partial error should keep prev, got %+v", p)
	}
	c.lastSampleAt = time.Now().Add(-30 * time.Second)
	p := c.Sample()
	if p == nil || len(p.Devices) != 1 {
		t.Fatalf("payload=%+v", p)
	}
	if p.Devices[0].RxBytes != 400 {
		t.Fatalf("want delta from original baseline, got %+v", p.Devices[0])
	}
}

func TestSampleNewMACNoPhantom(t *testing.T) {
	n := 0
	c := testCollector(t, func([]string) ([]wifiStation, error) {
		n++
		sts := []wifiStation{{MAC: "AA:BB:CC:DD:EE:01", Iface: "phy0-ap0", RxBytes: 1000, TxBytes: 2000}}
		if n > 1 {
			sts = append(sts, wifiStation{MAC: "AA:BB:CC:DD:EE:02", Iface: "phy0-ap0", RxBytes: 99999999, TxBytes: 88888888})
			sts[0].RxBytes = 1500
			sts[0].TxBytes = 2100
		}
		return sts, nil
	})
	_ = c.Sample()
	p := c.Sample()
	if p == nil || len(p.Devices) != 1 {
		t.Fatalf("devices=%v", p)
	}
	if p.Devices[0].MAC != "AA:BB:CC:DD:EE:01" {
		t.Fatalf("mac=%s", p.Devices[0].MAC)
	}
	if p.Devices[0].RxBytes != 500 || p.Devices[0].TxBytes != 100 {
		t.Fatalf("delta %+v", p.Devices[0])
	}
}

func TestSampleRoamNoBurst(t *testing.T) {
	n := 0
	c := testCollector(t, func([]string) ([]wifiStation, error) {
		n++
		if n == 1 {
			return []wifiStation{{MAC: "AA:BB:CC:DD:EE:01", Iface: "phy0-ap0", RxBytes: 9_000_000, TxBytes: 8_000_000}}, nil
		}
		return []wifiStation{{MAC: "AA:BB:CC:DD:EE:01", Iface: "phy1-ap0", RxBytes: 100, TxBytes: 50}}, nil
	})
	_ = c.Sample()
	p := c.Sample()
	if p == nil {
		t.Fatal("nil payload")
	}
	if len(p.Devices) != 0 {
		t.Fatalf("roam should not emit burst, got %+v", p.Devices)
	}
	if c.prev["AA:BB:CC:DD:EE:01"].Iface != "phy1-ap0" {
		t.Fatalf("prev iface=%s", c.prev["AA:BB:CC:DD:EE:01"].Iface)
	}
}

func TestSampleIdleHeartbeatPayload(t *testing.T) {
	c := testCollector(t, func([]string) ([]wifiStation, error) {
		return []wifiStation{{MAC: "AA:BB:CC:DD:EE:01", Iface: "phy0-ap0", RxBytes: 10, TxBytes: 10}}, nil
	})
	_ = c.Sample()
	p := c.Sample()
	if p == nil {
		t.Fatal("nil")
	}
	if len(p.Devices) != 0 {
		t.Fatalf("idle devices=%v", p.Devices)
	}
	if p.RouterID != "t" || p.IntervalSec < 1 {
		t.Fatalf("payload=%+v", p)
	}
}

func TestStringsToUpperMACRejectsJunk(t *testing.T) {
	if stringsToUpperMAC("not-a-mac-address") != "" {
		t.Fatal("17-byte junk")
	}
	if stringsToUpperMAC("aa:bb:cc:dd:ee:ff") != "AA:BB:CC:DD:EE:FF" {
		t.Fatal("normalize")
	}
}
