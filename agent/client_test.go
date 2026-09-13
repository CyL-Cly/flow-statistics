package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEnqueueHeartbeatWhenIdle(t *testing.T) {
	r := newReporter(Config{QueueMax: 8, HTTPTimeout: time.Second, Token: "t", ServerURL: "http://127.0.0.1/"})
	empty := ReportPayload{RouterID: "r"}
	r.Enqueue(empty)
	if r.queue.Len() != 1 {
		t.Fatalf("heartbeat not queued, len=%d", r.queue.Len())
	}
	r.Enqueue(ReportPayload{RouterID: "r", Devices: []DeviceReport{{MAC: "AA:BB:CC:DD:EE:01", RxBytes: 1}}})
	r.Enqueue(empty)
	if r.queue.Len() != 2 {
		t.Fatalf("empty should be skipped while queue busy, len=%d", r.queue.Len())
	}
}

func TestFlushForceIgnoresBackoff(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rep := newReporter(Config{QueueMax: 8, HTTPTimeout: time.Second, Token: "secret", ServerURL: srv.URL})
	rep.Enqueue(ReportPayload{RouterID: "r", Devices: []DeviceReport{{MAC: "AA:BB:CC:DD:EE:01", RxBytes: 1}}})
	rep.nextTry = time.Now().Add(time.Hour)
	rep.Flush(false)
	if hits != 0 {
		t.Fatal("backoff should skip")
	}
	rep.Flush(true)
	if hits != 1 {
		t.Fatalf("force flush hits=%d", hits)
	}
}

func TestFlushForceDrainsQueue(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rep := newReporter(Config{QueueMax: 200, HTTPTimeout: time.Second, Token: "secret", ServerURL: srv.URL})
	for i := 0; i < flushBatchSize+3; i++ {
		rep.Enqueue(ReportPayload{RouterID: "r", Timestamp: int64(i + 1), Devices: []DeviceReport{{MAC: "AA:BB:CC:DD:EE:01", RxBytes: 1}}})
	}
	rep.Flush(true)
	if hits != 2 {
		t.Fatalf("force drain hits=%d want 2", hits)
	}
	if rep.queue.Len() != 0 {
		t.Fatalf("left=%d", rep.queue.Len())
	}
}

func TestPostDoesNotFollowRedirect(t *testing.T) {
	var sawTokenOnRedirect bool
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Device-Token") != "" {
			sawTokenOnRedirect = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer final.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer origin.Close()

	rep := newReporter(Config{QueueMax: 8, HTTPTimeout: time.Second, Token: "secret", ServerURL: origin.URL})
	err := rep.post([]byte(`{"router_id":"r"}`))
	if err == nil {
		t.Fatal("expected redirect to be treated as failure")
	}
	if sawTokenOnRedirect {
		t.Fatal("token forwarded to redirect target")
	}
}

func TestPostSetsToken(t *testing.T) {
	var token, method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		token = r.Header.Get("X-Device-Token")
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	rep := newReporter(Config{HTTPTimeout: time.Second, Token: "abc", ServerURL: srv.URL})
	if err := rep.post([]byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || token != "abc" {
		t.Fatalf("method=%s token=%s", method, token)
	}
}

func TestQueuePrependKeepsNewest(t *testing.T) {
	q := newReportQueue(3)
	q.Push(ReportPayload{Timestamp: 1})
	q.Push(ReportPayload{Timestamp: 2})
	q.Prepend([]ReportPayload{{Timestamp: 8}, {Timestamp: 9}, {Timestamp: 10}, {Timestamp: 11}})
	if q.Len() != 3 {
		t.Fatalf("len=%d", q.Len())
	}
	batch := q.PopBatch(10)
	// combined=[8,9,10,11,1,2] truncated to newest 3 → [11,1,2]
	if batch[0].Timestamp != 11 || batch[1].Timestamp != 1 || batch[2].Timestamp != 2 {
		t.Fatalf("batch=%v", batch)
	}
}

func TestNewReportQueueDefault(t *testing.T) {
	q := newReportQueue(0)
	if q.max != defaultQueueMax {
		t.Fatalf("max=%d", q.max)
	}
}
