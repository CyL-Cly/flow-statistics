package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const flushBatchSize = 60

// Reporter posts samples to the public server with offline FIFO + backoff.
// Flush must only be called from a single goroutine (see failStreak/nextTry).
type Reporter struct {
	cfg    Config
	client *http.Client
	queue  *ReportQueue

	failStreak int
	nextTry    time.Time
}

func newReporter(cfg Config) *Reporter {
	return &Reporter{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.HTTPTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				// Do not forward X-Device-Token across 30x (Go only strips Authorization).
				return http.ErrUseLastResponse
			},
		},
		queue: newReportQueue(cfg.QueueMax),
	}
}

// Enqueue stores a sample for (re)transmission.
// Empty device lists are heartbeats: queued only when nothing else is waiting.
func (r *Reporter) Enqueue(p ReportPayload) {
	if len(p.Devices) == 0 && r.queue.Len() > 0 {
		return
	}
	r.queue.Push(p)
}

// Flush attempts to send queued reports (batch when multiple).
// force ignores backoff and drains until empty or a send fails.
func (r *Reporter) Flush(force bool) {
	for {
		if !force && time.Now().Before(r.nextTry) {
			return
		}
		if r.queue.Len() == 0 {
			return
		}

		batch := r.queue.PopBatch(flushBatchSize)
		if len(batch) == 0 {
			return
		}

		var (
			body []byte
			err  error
		)
		if len(batch) == 1 {
			body, err = json.Marshal(batch[0])
		} else {
			body, err = json.Marshal(batch)
		}
		if err != nil {
			log.Printf("marshal report: %v", err)
			r.queue.Prepend(batch)
			return
		}

		if err := r.post(body); err != nil {
			r.failStreak++
			backoff := backoffDuration(r.failStreak)
			r.nextTry = time.Now().Add(backoff)
			log.Printf("report failed (streak=%d backoff=%s): %v; re-queue %d", r.failStreak, backoff, err, len(batch))
			r.queue.Prepend(batch)
			return
		}

		r.failStreak = 0
		r.nextTry = time.Time{}
		if r.cfg.Verbose {
			if left := r.queue.Len(); left > 0 {
				log.Printf("report ok, %d still queued", left)
			}
		}
		if !force {
			return
		}
	}
}

func (r *Reporter) post(body []byte) error {
	req, err := http.NewRequest(http.MethodPost, r.cfg.ServerURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Device-Token", r.cfg.Token)

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	return nil
}

func backoffDuration(streak int) time.Duration {
	// 2s, 4s, 8s ... cap 256s
	d := time.Second * time.Duration(1<<min(streak, 8))
	if d < 2*time.Second {
		d = 2 * time.Second
	}
	return d
}
