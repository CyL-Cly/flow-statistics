package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Sample shape taken from live ImmortalWrt: iw dev phy1-ap0 station dump
func TestParseIWStationDump(t *testing.T) {
	raw := []byte(`Station 50:cf:56:f6:9c:b1 (on phy0-ap0)
		inactive time:	80 ms
		rx bytes:	1163449
		rx packets:	1828
		tx bytes:	998877
		tx packets:	1500
		tx retries:	3
		tx failed:	0
		signal:  	-55 [-55] dBm
	Station aa:bb:cc:dd:ee:ff (on phy0-ap0)
		inactive time:	1000 ms
		rx bytes:	100
		tx bytes:	200
	`)
	sts := parseIWStationDump(raw, "phy0-ap0")
	if len(sts) != 2 {
		t.Fatalf("stations=%d", len(sts))
	}
	if sts[0].MAC != "50:CF:56:F6:9C:B1" {
		t.Fatalf("mac=%s", sts[0].MAC)
	}
	// iw rx=1163449 → upload, iw tx=998877 → download
	if sts[0].RxBytes != 998877 || sts[0].TxBytes != 1163449 {
		t.Fatalf("bytes rx=%d tx=%d", sts[0].RxBytes, sts[0].TxBytes)
	}
	if sts[1].RxBytes != 200 || sts[1].TxBytes != 100 {
		t.Fatalf("sta2 bytes rx=%d tx=%d", sts[1].RxBytes, sts[1].TxBytes)
	}
}

func TestParseIWIntRejectsGarbage(t *testing.T) {
	if _, ok := parseIWInt("rx bytes: not-a-number"); ok {
		t.Fatal("garbage")
	}
	n, ok := parseIWInt("tx bytes:	42 extra")
	if !ok || n != 42 {
		t.Fatalf("n=%d ok=%v", n, ok)
	}
}

func TestParseIWDevAPOnly(t *testing.T) {
	raw := []byte(`phy#0
	Interface phy0-ap0
		ifindex 15
		wdev 0x2
		addr 00:11:22:33:44:55
		ssid home
		type AP
		channel 36 (5180 MHz), width: 80 MHz
	Interface phy0-sta0
		ifindex 16
		type managed
	Interface phy1-ap0
		type AP
	Interface wlan0
		type AP-VLAN
`)
	got := parseIWDev(raw)
	if len(got) != 3 {
		t.Fatalf("got=%v", got)
	}
	if got[0] != "phy0-ap0" || got[1] != "phy1-ap0" || got[2] != "wlan0" {
		t.Fatalf("got=%v", got)
	}
}

func TestValidNetIface(t *testing.T) {
	if !validNetIface("phy0-ap0") || !validNetIface("wlan0") {
		t.Fatal("ok names")
	}
	if validNetIface("-v") || validNetIface(".hidden") || validNetIface("phy0 ap0") || validNetIface("") {
		t.Fatal("bad names")
	}
}

func TestEntryDelta(t *testing.T) {
	if entryDelta(100, 40) != 60 {
		t.Fatal("growth")
	}
	if entryDelta(10, 100) != 10 {
		t.Fatal("reset should take cur")
	}
	if entryDelta(0, 50) != 0 {
		t.Fatal("zero after reset")
	}
}

func TestQueueFIFO(t *testing.T) {
	q := newReportQueue(3)
	for i := 1; i <= 4; i++ {
		q.Push(ReportPayload{Timestamp: int64(i)})
	}
	if q.Len() != 3 {
		t.Fatalf("len=%d", q.Len())
	}
	batch := q.PopBatch(10)
	if len(batch) != 3 || batch[0].Timestamp != 2 || batch[2].Timestamp != 4 {
		t.Fatalf("batch=%v", batch)
	}
}

func TestMetaDHCP(t *testing.T) {
	dir := t.TempDir()
	leases := filepath.Join(dir, "leases")
	content := "1710000000 50:cf:56:f6:9c:b1 192.168.50.145 redmi *\n"
	if err := os.WriteFile(leases, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newMetaCache(leases, filepath.Join(dir, "arp"))
	meta := m.ResolveByMAC("50:CF:56:F6:9C:B1", "phy0-ap0")
	if meta.IP != "192.168.50.145" || meta.Name != "redmi" {
		t.Fatalf("meta=%+v", meta)
	}
}

func TestMetaDHCPRejectsJunk(t *testing.T) {
	dir := t.TempDir()
	leases := filepath.Join(dir, "leases")
	content := "1710000000 not-a-mac 192.168.50.1 bad *\n1710000000 aa:bb:cc:dd:ee:ff not-an-ip host *\n"
	if err := os.WriteFile(leases, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newMetaCache(leases, filepath.Join(dir, "arp"))
	meta := m.ResolveByMAC("AA:BB:CC:DD:EE:FF", "phy0-ap0")
	if meta.IP != "" {
		t.Fatalf("expected empty IP, got %+v", meta)
	}
}
