package redisqueue

import (
	"strconv"
	"testing"
	"time"
)

func TestQueueEnqueueCapsRetainedItems(t *testing.T) {
	q := &Queue{enabled: true}
	for i := 0; i < maxQueueItems+10; i++ {
		q.Enqueue([]byte(strconv.Itoa(i)))
	}

	items := q.PopOldest(maxQueueItems + 20)
	if len(items) != maxQueueItems {
		t.Fatalf("popOldest() item count = %d, want %d", len(items), maxQueueItems)
	}
	if string(items[0]) != "10" {
		t.Fatalf("oldest retained payload = %q, want %q", string(items[0]), "10")
	}
	if string(items[len(items)-1]) != strconv.Itoa(maxQueueItems+9) {
		t.Fatalf("newest retained payload = %q, want %q", string(items[len(items)-1]), strconv.Itoa(maxQueueItems+9))
	}
}

func TestQueuePruneExpiredDropsStaleItems(t *testing.T) {
	now := time.Now()
	q := &Queue{
		enabled: true,
		items: []queueItem{
			{enqueuedAt: now.Add(-retentionWindow - time.Second), payload: []byte("stale")},
			{enqueuedAt: now, payload: []byte("fresh")},
		},
	}

	q.pruneExpired(now)
	items := q.PopOldest(10)
	if len(items) != 1 {
		t.Fatalf("popOldest() item count = %d, want 1", len(items))
	}
	if string(items[0]) != "fresh" {
		t.Fatalf("retained payload = %q, want %q", string(items[0]), "fresh")
	}
}

func TestQueuePopNewestReturnsRightmostItemsFirst(t *testing.T) {
	q := &Queue{enabled: true}
	q.Enqueue([]byte("a"))
	q.Enqueue([]byte("b"))
	q.Enqueue([]byte("c"))

	items := q.PopNewest(2)
	if len(items) != 2 {
		t.Fatalf("popNewest() item count = %d, want 2", len(items))
	}
	if string(items[0]) != "c" {
		t.Fatalf("first popped payload = %q, want %q", string(items[0]), "c")
	}
	if string(items[1]) != "b" {
		t.Fatalf("second popped payload = %q, want %q", string(items[1]), "b")
	}

	remaining := q.PopOldest(10)
	if len(remaining) != 1 || string(remaining[0]) != "a" {
		t.Fatalf("remaining payloads = %#v, want [a]", remaining)
	}
}
