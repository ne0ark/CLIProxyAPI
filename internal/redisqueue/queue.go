package redisqueue

import (
	"context"
	"sync"
	"time"
)

const (
	retentionWindow = time.Minute
	maxQueueItems   = 4096
	janitorInterval = 15 * time.Second
)

type queueItem struct {
	enqueuedAt time.Time
	payload    []byte
}

type queueContextKey struct{}

type Queue struct {
	mu               sync.Mutex
	enabled          bool
	recordingEnabled bool
	items            []queueItem
	head             int
	janitorMu        sync.Mutex
	janitorCh        chan struct{}
}

var global = Queue{recordingEnabled: true}

func NewQueue() *Queue {
	return &Queue{recordingEnabled: true}
}

func DefaultQueue() *Queue {
	return &global
}

func WithQueue(ctx context.Context, queue *Queue) context.Context {
	if queue == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, queueContextKey{}, queue)
}

func FromContext(ctx context.Context) *Queue {
	if ctx == nil {
		return nil
	}
	queue, _ := ctx.Value(queueContextKey{}).(*Queue)
	return queue
}

func SetEnabled(value bool) {
	global.SetEnabled(value)
}

func Enabled() bool {
	return global.Enabled()
}

func Enqueue(payload []byte) {
	global.Enqueue(payload)
}

func PopOldest(count int) [][]byte {
	return global.PopOldest(count)
}

func PopNewest(count int) [][]byte {
	return global.PopNewest(count)
}

func (q *Queue) SetEnabled(value bool) {
	if q == nil {
		return
	}
	q.mu.Lock()
	q.enabled = value
	if !value {
		q.items = nil
		q.head = 0
	}
	q.mu.Unlock()
	if value {
		q.startJanitor()
		return
	}
	q.stopJanitor()
}

func (q *Queue) Enabled() bool {
	if q == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.enabled
}

func (q *Queue) SetRecordingEnabled(value bool) {
	if q == nil {
		return
	}
	q.mu.Lock()
	q.recordingEnabled = value
	q.mu.Unlock()
}

func (q *Queue) RecordingEnabled() bool {
	if q == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.recordingEnabled
}

func (q *Queue) Enqueue(payload []byte) {
	if q == nil || len(payload) == 0 {
		return
	}
	now := time.Now()

	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.enabled {
		return
	}

	q.pruneLocked(now)
	q.items = append(q.items, queueItem{
		enqueuedAt: now,
		payload:    append([]byte(nil), payload...),
	})
	q.trimExcessLocked()
	q.maybeCompactLocked()
}

func (q *Queue) PopOldest(count int) [][]byte {
	if q == nil || count <= 0 {
		return nil
	}
	now := time.Now()

	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.enabled {
		return nil
	}

	q.pruneLocked(now)
	available := len(q.items) - q.head
	if available <= 0 {
		q.items = nil
		q.head = 0
		return nil
	}
	if count > available {
		count = available
	}

	out := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		item := q.items[q.head+i]
		out = append(out, item.payload)
	}
	q.head += count
	q.maybeCompactLocked()
	return out
}

func (q *Queue) PopNewest(count int) [][]byte {
	if q == nil || count <= 0 {
		return nil
	}
	now := time.Now()

	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.enabled {
		return nil
	}

	q.pruneLocked(now)
	available := len(q.items) - q.head
	if available <= 0 {
		q.items = nil
		q.head = 0
		return nil
	}
	if count > available {
		count = available
	}

	start := len(q.items) - count
	out := make([][]byte, 0, count)
	for i := len(q.items) - 1; i >= start; i-- {
		out = append(out, q.items[i].payload)
	}
	q.items = q.items[:start]
	q.maybeCompactLocked()
	return out
}

func (q *Queue) pruneLocked(now time.Time) {
	if q.head >= len(q.items) {
		q.items = nil
		q.head = 0
		return
	}

	cutoff := now.Add(-retentionWindow)
	for q.head < len(q.items) && q.items[q.head].enqueuedAt.Before(cutoff) {
		q.head++
	}
}

func (q *Queue) pruneExpired(now time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pruneLocked(now)
	q.maybeCompactLocked()
}

func (q *Queue) maybeCompactLocked() {
	if q.head == 0 {
		return
	}
	if q.head >= len(q.items) {
		q.items = nil
		q.head = 0
		return
	}
	if q.head < 1024 && q.head*2 < len(q.items) {
		return
	}
	q.items = append([]queueItem(nil), q.items[q.head:]...)
	q.head = 0
}

func (q *Queue) trimExcessLocked() {
	available := len(q.items) - q.head
	if available <= maxQueueItems {
		return
	}
	q.head += available - maxQueueItems
}

func (q *Queue) startJanitor() {
	if q == nil {
		return
	}
	q.janitorMu.Lock()
	defer q.janitorMu.Unlock()
	if q.janitorCh != nil {
		return
	}
	stopCh := make(chan struct{})
	q.janitorCh = stopCh
	go q.runJanitor(stopCh)
}

func (q *Queue) stopJanitor() {
	if q == nil {
		return
	}
	q.janitorMu.Lock()
	stopCh := q.janitorCh
	q.janitorCh = nil
	q.janitorMu.Unlock()
	if stopCh != nil {
		close(stopCh)
	}
}

func (q *Queue) runJanitor(stopCh <-chan struct{}) {
	ticker := time.NewTicker(janitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			q.pruneExpired(time.Now())
		case <-stopCh:
			return
		}
	}
}
