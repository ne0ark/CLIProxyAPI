package usage

import (
	"context"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRequestStatisticsRecordIncludesLatency(t *testing.T) {
	stats := NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "test-key",
		Model:       "gpt-5.4",
		RequestedAt: time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		Latency:     1500 * time.Millisecond,
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	})

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["gpt-5.4"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
	if details[0].LatencyMs != 1500 {
		t.Fatalf("latency_ms = %d, want 1500", details[0].LatencyMs)
	}
}

func TestRequestStatisticsMergeSnapshotDedupIgnoresLatency(t *testing.T) {
	stats := NewRequestStatistics()
	timestamp := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	first := StatisticsSnapshot{
		APIs: map[string]APISnapshot{
			"test-key": {
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						Details: []RequestDetail{{
							Timestamp: timestamp,
							LatencyMs: 0,
							Source:    "user@example.com",
							AuthIndex: "0",
							Tokens: TokenStats{
								InputTokens:  10,
								OutputTokens: 20,
								TotalTokens:  30,
							},
						}},
					},
				},
			},
		},
	}
	second := StatisticsSnapshot{
		APIs: map[string]APISnapshot{
			"test-key": {
				Models: map[string]ModelSnapshot{
					"gpt-5.4": {
						Details: []RequestDetail{{
							Timestamp: timestamp,
							LatencyMs: 2500,
							Source:    "user@example.com",
							AuthIndex: "0",
							Tokens: TokenStats{
								InputTokens:  10,
								OutputTokens: 20,
								TotalTokens:  30,
							},
						}},
					},
				},
			},
		},
	}

	result := stats.MergeSnapshot(first)
	if result.Added != 1 || result.Skipped != 0 {
		t.Fatalf("first merge = %+v, want added=1 skipped=0", result)
	}

	result = stats.MergeSnapshot(second)
	if result.Added != 0 || result.Skipped != 1 {
		t.Fatalf("second merge = %+v, want added=0 skipped=1", result)
	}

	snapshot := stats.Snapshot()
	details := snapshot.APIs["test-key"].Models["gpt-5.4"].Details
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
}

func TestRequestStatisticsRecordAdditionalModelDoesNotIncrementTopLevelRequests(t *testing.T) {
	stats := NewRequestStatistics()
	timestamp := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)

	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "test-key",
		Model:       "gpt-5.4",
		RequestedAt: timestamp,
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	})
	stats.Record(context.Background(), coreusage.Record{
		APIKey:          "test-key",
		Model:           "gpt-image-2",
		RequestedAt:     timestamp,
		AdditionalModel: true,
		Detail: coreusage.Detail{
			InputTokens: 4,
			TotalTokens: 4,
		},
	})

	snapshot := stats.Snapshot()
	if snapshot.TotalRequests != 1 {
		t.Fatalf("snapshot.TotalRequests = %d, want 1", snapshot.TotalRequests)
	}
	if snapshot.TotalTokens != 34 {
		t.Fatalf("snapshot.TotalTokens = %d, want 34", snapshot.TotalTokens)
	}
	api := snapshot.APIs["test-key"]
	if api.TotalRequests != 1 {
		t.Fatalf("api.TotalRequests = %d, want 1", api.TotalRequests)
	}
	if api.TotalTokens != 34 {
		t.Fatalf("api.TotalTokens = %d, want 34", api.TotalTokens)
	}
	if api.Models["gpt-5.4"].TotalRequests != 1 {
		t.Fatalf("primary model TotalRequests = %d, want 1", api.Models["gpt-5.4"].TotalRequests)
	}
	if api.Models["gpt-image-2"].TotalRequests != 1 {
		t.Fatalf("additional model TotalRequests = %d, want 1", api.Models["gpt-image-2"].TotalRequests)
	}
	if got := snapshot.RequestsByDay["2026-03-20"]; got != 1 {
		t.Fatalf("requestsByDay = %d, want 1", got)
	}
	if got := snapshot.TokensByDay["2026-03-20"]; got != 34 {
		t.Fatalf("tokensByDay = %d, want 34", got)
	}
}
