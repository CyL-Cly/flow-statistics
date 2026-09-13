package main

// ReportPayload matches the server API.
type ReportPayload struct {
	RouterID    string         `json:"router_id"`
	Timestamp   int64          `json:"timestamp"`
	IntervalSec int            `json:"interval_sec"`
	Devices     []DeviceReport `json:"devices"`
}

// DeviceReport is one Wi-Fi station sample (delta bytes over the interval).
// RxBytes = download (AP→client), TxBytes = upload (client→AP).
type DeviceReport struct {
	MAC     string `json:"mac"`
	IP      string `json:"ip"`
	Name    string `json:"name"`
	Iface   string `json:"iface"`
	RxBytes int64  `json:"rx_bytes"`
	TxBytes int64  `json:"tx_bytes"`
}

// deviceMeta is hostname / mac / iface enrichment for a station.
type deviceMeta struct {
	IP    string
	MAC   string
	Name  string
	Iface string
}
