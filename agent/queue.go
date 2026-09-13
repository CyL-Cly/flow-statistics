package main

import "sync"

// ReportQueue is a bounded in-memory FIFO for offline / failed reports.
type ReportQueue struct {
	mu   sync.Mutex
	max  int
	data []ReportPayload
}

func newReportQueue(max int) *ReportQueue {
	if max <= 0 {
		max = defaultQueueMax
	}
	return &ReportQueue{max: max, data: make([]ReportPayload, 0, 64)}
}

// Push appends a report; drops oldest when full.
func (q *ReportQueue) Push(p ReportPayload) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.data) >= q.max {
		copy(q.data[0:], q.data[1:])
		q.data = q.data[:len(q.data)-1]
	}
	q.data = append(q.data, p)
}

// Len returns queued count.
func (q *ReportQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.data)
}

// PopBatch removes and returns up to n items (FIFO order).
func (q *ReportQueue) PopBatch(n int) []ReportPayload {
	q.mu.Lock()
	defer q.mu.Unlock()
	if n <= 0 || len(q.data) == 0 {
		return nil
	}
	if n > len(q.data) {
		n = len(q.data)
	}
	out := make([]ReportPayload, n)
	copy(out, q.data[:n])
	q.data = q.data[n:]
	if cap(q.data) > 256 && len(q.data) < cap(q.data)/4 {
		n2 := make([]ReportPayload, len(q.data), len(q.data)+16)
		copy(n2, q.data)
		q.data = n2
	}
	return out
}

// Prepend re-queues items at the front (failed batch restore).
func (q *ReportQueue) Prepend(items []ReportPayload) {
	if len(items) == 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	combined := make([]ReportPayload, 0, len(items)+len(q.data))
	combined = append(combined, items...)
	combined = append(combined, q.data...)
	if len(combined) > q.max {
		combined = combined[len(combined)-q.max:]
	}
	q.data = combined
}
