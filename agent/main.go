// OpenWrt traffic agent: sample Wi-Fi station counters and report deltas to server.
//
// Build for OpenWrt aarch64:
//
//	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o traffic-agent .
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	cfg := loadConfig()
	log.Printf("traffic-agent start router_id=%s interval=%s server=%s source=wifi-station verbose=%v",
		cfg.RouterID, cfg.Interval, cfg.ServerURL, cfg.Verbose)

	col := newCollector(cfg)
	rep := newReporter(cfg)

	interval := cfg.Interval
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()

	flushTicker := time.NewTicker(time.Second)
	defer flushTicker.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	// initial baseline sample
	_ = col.Sample()

	for {
		select {
		case <-t.C:
			if p := col.Sample(); p != nil {
				rep.Enqueue(*p)
				rep.Flush()
			}
		case <-flushTicker.C:
			if rep.queue.Len() > 0 {
				rep.Flush()
			}
		case s := <-sig:
			log.Printf("signal %v, exit", s)
			rep.Flush()
			return
		}
	}
}
