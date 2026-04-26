package test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	runtimeexecutor "github.com/router-for-me/CLIProxyAPI/v6/internal/runtime/executor"
	internalusage "github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGeminiExecutorRecordsSuccessfulZeroUsageInStatistics(t *testing.T) {
	model := fmt.Sprintf("gemini-2.5-flash-zero-usage-%d", time.Now().UnixNano())
	source := fmt.Sprintf("zero-usage-%d@example.com", time.Now().UnixNano())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/v1beta/models/" + model + ":generateContent"
		if r.URL.Path != wantPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, wantPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":0,"candidatesTokenCount":0,"totalTokenCount":0}}`))
	}))
	defer server.Close()

	executor := runtimeexecutor.NewGeminiExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		Provider: "gemini",
		Attributes: map[string]string{
			"api_key":  "test-upstream-key",
			"base_url": server.URL,
		},
		Metadata: map[string]any{
			"email": source,
		},
	}

	prevStatsEnabled := internalusage.StatisticsEnabled()
	internalusage.SetStatisticsEnabled(true)
	t.Cleanup(func() {
		internalusage.SetStatisticsEnabled(prevStatsEnabled)
	})

	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   model,
		Payload: []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`),
	}, cliproxyexecutor.Options{
		SourceFormat:    sdktranslator.FormatGemini,
		OriginalRequest: []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`),
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	detail := waitForStatisticsDetail(t, "gemini", model, source)
	if detail.Failed {
		t.Fatalf("detail failed = true, want false")
	}
	if detail.Tokens.TotalTokens != 0 {
		t.Fatalf("total tokens = %d, want 0", detail.Tokens.TotalTokens)
	}
}

func TestGeminiCLIExecutorStreamRecordsSuccessfulZeroUsageInStatistics(t *testing.T) {
	model := fmt.Sprintf("gemini-2.5-pro-zero-usage-%d", time.Now().UnixNano())
	source := fmt.Sprintf("gemini-cli-zero-usage-%d@example.com", time.Now().UnixNano())
	payload := []byte(`{"request":{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}}`)

	ctx := context.WithValue(context.Background(), "cliproxy.roundtripper", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "cloudcode-pa.googleapis.com" {
			t.Fatalf("host = %q, want %q", req.URL.Host, "cloudcode-pa.googleapis.com")
		}
		if req.URL.Path != "/v1internal:streamGenerateContent" {
			t.Fatalf("path = %q, want %q", req.URL.Path, "/v1internal:streamGenerateContent")
		}
		if req.URL.RawQuery != "alt=sse" {
			t.Fatalf("raw query = %q, want %q", req.URL.RawQuery, "alt=sse")
		}
		if authz := req.Header.Get("Authorization"); authz != "Bearer test-access-token" {
			t.Fatalf("authorization = %q, want %q", authz, "Bearer test-access-token")
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if !strings.Contains(string(body), `"`+"model"+`":"`+model+`"`) {
			t.Fatalf("request body missing model %q: %s", model, string(body))
		}

		stream := strings.Join([]string{
			`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}]}}`,
			"",
			`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":" done"}]},"finishReason":"STOP"}]}}`,
			"",
		}, "\n")

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(stream)),
		}, nil
	}))

	executor := runtimeexecutor.NewGeminiCLIExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		ID:       "gemini-cli-zero-usage-auth",
		Provider: "gemini-cli",
		Metadata: map[string]any{
			"access_token": "test-access-token",
			"token_type":   "Bearer",
			"expiry":       time.Now().Add(time.Hour).Format(time.RFC3339),
			"email":        source,
		},
	}

	prevStatsEnabled := internalusage.StatisticsEnabled()
	internalusage.SetStatisticsEnabled(true)
	t.Cleanup(func() {
		internalusage.SetStatisticsEnabled(prevStatsEnabled)
	})

	stream, err := executor.ExecuteStream(ctx, auth, cliproxyexecutor.Request{
		Model:   model,
		Payload: payload,
	}, cliproxyexecutor.Options{
		Stream:          true,
		SourceFormat:    sdktranslator.FormatGeminiCLI,
		OriginalRequest: payload,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}

	for chunk := range stream.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error: %v", chunk.Err)
		}
	}

	detail := waitForStatisticsDetail(t, "gemini-cli", model, auth.ID)
	if detail.Failed {
		t.Fatalf("detail failed = true, want false")
	}
	if detail.Tokens.TotalTokens != 0 {
		t.Fatalf("total tokens = %d, want 0", detail.Tokens.TotalTokens)
	}
}

func waitForStatisticsDetail(t *testing.T, apiName, model, source string) internalusage.RequestDetail {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := internalusage.GetRequestStatistics().Snapshot()
		apiSnapshot, ok := snapshot.APIs[apiName]
		if !ok {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		modelSnapshot, ok := apiSnapshot.Models[model]
		if !ok {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		for _, detail := range modelSnapshot.Details {
			if detail.Source == source {
				return detail
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for statistics detail for api=%q model=%q source=%q", apiName, model, source)
	return internalusage.RequestDetail{}
}
