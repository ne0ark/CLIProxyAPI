package executor

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	_ "github.com/router-for-me/CLIProxyAPI/v6/internal/translator"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestCodexExecutorExecute_EmptyStreamCompletionOutputUsesOutputItemDone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]},\"output_index\":0}\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":1775555723,\"status\":\"completed\",\"model\":\"gpt-5.4-mini-2026-03-17\",\"output\":[],\"usage\":{\"input_tokens\":8,\"output_tokens\":28,\"total_tokens\":36}}}\n\n"))
	}))
	defer server.Close()

	executor := NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test",
	}}

	resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.4-mini",
		Payload: []byte(`{"model":"gpt-5.4-mini","messages":[{"role":"user","content":"Say ok"}]}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       false,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	gotContent := gjson.GetBytes(resp.Payload, "choices.0.message.content").String()
	if gotContent != "ok" {
		t.Fatalf("choices.0.message.content = %q, want %q; payload=%s", gotContent, "ok", string(resp.Payload))
	}
}

func TestPatchCodexCompletedOutput_PreservesIndexedOrderAndAuthoritativeOutput(t *testing.T) {
	indexed := map[int64][]byte{
		2: []byte(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"third"}]}`),
		0: []byte(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"first"}]}`),
	}
	fallback := [][]byte{
		[]byte(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"fallback"}]}`),
	}

	patched := patchCodexCompletedOutput([]byte(`{"type":"response.completed","response":{"output":[]}}`), indexed, fallback)
	gotItems := gjson.GetBytes(patched, "response.output").Array()
	if len(gotItems) != 3 {
		t.Fatalf("len(response.output) = %d, want 3; payload=%s", len(gotItems), string(patched))
	}

	gotTexts := []string{
		gotItems[0].Get("content.0.text").String(),
		gotItems[1].Get("content.0.text").String(),
		gotItems[2].Get("content.0.text").String(),
	}
	wantTexts := []string{"first", "third", "fallback"}
	for i := range wantTexts {
		if gotTexts[i] != wantTexts[i] {
			t.Fatalf("response.output[%d].content[0].text = %q, want %q; payload=%s", i, gotTexts[i], wantTexts[i], string(patched))
		}
	}

	authoritative := []byte(`{"type":"response.completed","response":{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"authoritative"}]}]}}`)
	preserved := patchCodexCompletedOutput(authoritative, indexed, fallback)
	preservedItems := gjson.GetBytes(preserved, "response.output").Array()
	if len(preservedItems) != 1 {
		t.Fatalf("len(response.output) = %d, want 1 authoritative item; payload=%s", len(preservedItems), string(preserved))
	}
	if got := preservedItems[0].Get("content.0.text").String(); got != "authoritative" {
		t.Fatalf("response.output[0].content[0].text = %q, want %q; payload=%s", got, "authoritative", string(preserved))
	}
}

func TestCodexExecutorExecuteStream_EmptyStreamCompletionOutputUsesOutputItemDone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"third\"}]},\"output_index\":2}\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"first\"}]},\"output_index\":0}\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"fallback\"}]}}\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":1775555723,\"status\":\"completed\",\"model\":\"gpt-5.4-mini-2026-03-17\",\"output\":[],\"usage\":{\"input_tokens\":8,\"output_tokens\":28,\"total_tokens\":36}}}\n\n"))
	}))
	defer server.Close()

	executor := NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test",
	}}

	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.4-mini",
		Payload: []byte(`{"model":"gpt-5.4-mini","input":"Say ok"}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}

	var completed []byte
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error: %v", chunk.Err)
		}
		payload := bytes.TrimSpace(chunk.Payload)
		if !bytes.HasPrefix(payload, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(payload[5:])
		if gjson.GetBytes(data, "type").String() == "response.completed" {
			completed = append([]byte(nil), data...)
		}
	}

	if len(completed) == 0 {
		t.Fatal("missing response.completed chunk")
	}

	gotItems := gjson.GetBytes(completed, "response.output").Array()
	if len(gotItems) != 3 {
		t.Fatalf("len(response.output) = %d, want 3; completed=%s", len(gotItems), string(completed))
	}

	gotTexts := []string{
		gotItems[0].Get("content.0.text").String(),
		gotItems[1].Get("content.0.text").String(),
		gotItems[2].Get("content.0.text").String(),
	}
	wantTexts := []string{"first", "third", "fallback"}
	for i := range wantTexts {
		if gotTexts[i] != wantTexts[i] {
			t.Fatalf("response.output[%d].content[0].text = %q, want %q; completed=%s", i, gotTexts[i], wantTexts[i], string(completed))
		}
	}
}
