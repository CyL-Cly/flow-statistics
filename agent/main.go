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
	"strings"
	"sync"
	"syscall"
	"time"
)

// flushDeadline bounds the shutdown flush wait so a slow server cannot
// stall the OpenWrt reboot sequence indefinitely.
const flushDeadline = 15 * time.Second

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	for _, a := range os.Args[1:] {
		if a == "-token" || strings.HasPrefix(a, "-token=") {
			log.Printf("warning: -token is visible in process list; use TRAFFIC_TOKEN")
			break
		}
	}
	log.Printf("traffic-agent start router_id=%s interval=%s server=%s source=wifi-station verbose=%v",
		cfg.RouterID, cfg.Interval, cfg.ServerURL, cfg.Verbose)

	col := newCollector(cfg)
	rep := newReporter(cfg)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		flushTicker := time.NewTicker(time.Second)
		defer flushTicker.Stop()
		for {
			select {
			case <-flushTicker.C:
				rep.Flush(false)
			case <-stop:
				rep.Flush(true)
				return
			}
		}
	}()

	t := time.NewTicker(cfg.Interval)
	defer t.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	// initial baseline sample
	_ = col.Sample()

	for {
		select {
		case <-t.C:
			if p := col.Sample(); p != nil {
				rep.Enqueue(*p)
			}
		case s := <-sig:
			log.Printf("signal %v, exit", s)
			close(stop)
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()
			select {
			case <-done:
			case s2 := <-sig:
				log.Printf("signal %v, force exit", s2)
				os.Exit(0)
			case <-time.After(flushDeadline):
				log.Printf("flush deadline exceeded, %d report(s) unsent", rep.queue.Len())
				os.Exit(0)
			}
			return
		}
	}
}
