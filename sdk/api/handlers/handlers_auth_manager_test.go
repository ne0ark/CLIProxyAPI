package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

type authManagerNonStreamingTestExecutor struct {
	executeCalls int
	countCalls   int

	lastExecuteReq  coreexecutor.Request
	lastExecuteOpts coreexecutor.Options
	lastCountReq    coreexecutor.Request
	lastCountOpts   coreexecutor.Options
}

func (e *authManagerNonStreamingTestExecutor) Identifier() string { return "gemini" }

func (e *authManagerNonStreamingTestExecutor) Execute(_ context.Context, _ *coreauth.Auth, req coreexecutor.Request, opts coreexecutor.Options) (coreexecutor.Response, error) {
	e.executeCalls++
	e.lastExecuteReq = req
	e.lastExecuteOpts = opts
	return coreexecutor.Response{
		Payload: []byte("execute-response"),
		Headers: http.Header{
			"Set-Cookie":      []string{"drop-me"},
			"X-Test-Upstream": []string{"keep-me"},
		},
	}, nil
}

func (e *authManagerNonStreamingTestExecutor) ExecuteStream(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (*coreexecutor.StreamResult, error) {
	return nil, nil
}

func (e *authManagerNonStreamingTestExecutor) Refresh(_ context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	return auth, nil
}

func (e *authManagerNonStreamingTestExecutor) CountTokens(_ context.Context, _ *coreauth.Auth, req coreexecutor.Request, opts coreexecutor.Options) (coreexecutor.Response, error) {
	e.countCalls++
	e.lastCountReq = req
	e.lastCountOpts = opts
	return coreexecutor.Response{
		Payload: []byte("count-response"),
		Headers: http.Header{
			"Set-Cookie":      []string{"drop-me"},
			"X-Test-Upstream": []string{"keep-me"},
		},
	}, nil
}

func (e *authManagerNonStreamingTestExecutor) HttpRequest(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error) {
	return nil, nil
}

type authManagerStreamingTestExecutor struct {
	lastStreamReq  coreexecutor.Request
	lastStreamOpts coreexecutor.Options
}

func (e *authManagerStreamingTestExecutor) Identifier() string { return "openai" }

func (e *authManagerStreamingTestExecutor) Execute(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	return coreexecutor.Response{}, nil
}

func (e *authManagerStreamingTestExecutor) ExecuteStream(_ context.Context, _ *coreauth.Auth, req coreexecutor.Request, opts coreexecutor.Options) (*coreexecutor.StreamResult, error) {
	e.lastStreamReq = req
	e.lastStreamOpts = opts
	chunks := make(chan coreexecutor.StreamChunk)
	close(chunks)
	return &coreexecutor.StreamResult{Chunks: chunks}, nil
}

func (e *authManagerStreamingTestExecutor) Refresh(_ context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	return auth, nil
}

func (e *authManagerStreamingTestExecutor) CountTokens(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	return coreexecutor.Response{}, nil
}

func (e *authManagerStreamingTestExecutor) HttpRequest(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error) {
	return nil, nil
}

func TestExecuteNonStreamingWithAuthManager_SharedPath(t *testing.T) {
	modelRegistry := registry.GetGlobalRegistry()
	const authID = "test-handlers-auth-manager-shared-path-auth"
	modelRegistry.RegisterClient(authID, "gemini", []*registry.ModelInfo{
		{ID: "gemini-2.5-pro", Created: time.Now().Unix()},
	})
	t.Cleanup(func() {
		modelRegistry.UnregisterClient(authID)
	})

	tests := []struct {
		name             string
		call             func(*BaseAPIHandler, context.Context, string, string, []byte, string) ([]byte, http.Header, *interfaces.ErrorMessage)
		wantPayload      string
		wantExecuteCalls int
		wantCountCalls   int
		capturedReq      func(*authManagerNonStreamingTestExecutor) coreexecutor.Request
		capturedOpts     func(*authManagerNonStreamingTestExecutor) coreexecutor.Options
	}{
		{
			name:             "execute",
			call:             (*BaseAPIHandler).ExecuteWithAuthManager,
			wantPayload:      "execute-response",
			wantExecuteCalls: 1,
			wantCountCalls:   0,
			capturedReq: func(executor *authManagerNonStreamingTestExecutor) coreexecutor.Request {
				return executor.lastExecuteReq
			},
			capturedOpts: func(executor *authManagerNonStreamingTestExecutor) coreexecutor.Options {
				return executor.lastExecuteOpts
			},
		},
		{
			name:             "count",
			call:             (*BaseAPIHandler).ExecuteCountWithAuthManager,
			wantPayload:      "count-response",
			wantExecuteCalls: 0,
			wantCountCalls:   1,
			capturedReq: func(executor *authManagerNonStreamingTestExecutor) coreexecutor.Request {
				return executor.lastCountReq
			},
			capturedOpts: func(executor *authManagerNonStreamingTestExecutor) coreexecutor.Options {
				return executor.lastCountOpts
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := coreauth.NewManager(nil, nil, nil)
			executor := &authManagerNonStreamingTestExecutor{}
			manager.RegisterExecutor(executor)
			if _, err := manager.Register(context.Background(), &coreauth.Auth{
				ID:       authID,
				Provider: "gemini",
			}); err != nil {
				t.Fatalf("Register() error = %v", err)
			}

			handler := NewBaseAPIHandlers(&sdkconfig.SDKConfig{PassthroughHeaders: true}, manager)
			rawJSON := []byte(`{"model":"gemini-2.5-pro","contents":[]}`)

			payload, headers, errMsg := tt.call(handler, context.Background(), "openai", "gemini-2.5-pro", rawJSON, "responses/compact")
			if errMsg != nil {
				t.Fatalf("auth-manager execution error = %v", errMsg.Error)
			}
			if string(payload) != tt.wantPayload {
				t.Fatalf("payload = %q, want %q", string(payload), tt.wantPayload)
			}
			if got := headers.Get("X-Test-Upstream"); got != "keep-me" {
				t.Fatalf("X-Test-Upstream = %q, want %q", got, "keep-me")
			}
			if got := headers.Get("Set-Cookie"); got != "" {
				t.Fatalf("Set-Cookie = %q, want empty after filtering", got)
			}
			if executor.executeCalls != tt.wantExecuteCalls {
				t.Fatalf("executeCalls = %d, want %d", executor.executeCalls, tt.wantExecuteCalls)
			}
			if executor.countCalls != tt.wantCountCalls {
				t.Fatalf("countCalls = %d, want %d", executor.countCalls, tt.wantCountCalls)
			}

			req := tt.capturedReq(executor)
			if req.Model != "gemini-2.5-pro" {
				t.Fatalf("request model = %q, want %q", req.Model, "gemini-2.5-pro")
			}
			if string(req.Payload) != string(rawJSON) {
				t.Fatalf("request payload = %s, want %s", string(req.Payload), string(rawJSON))
			}

			opts := tt.capturedOpts(executor)
			if opts.Stream {
				t.Fatalf("opts.Stream = true, want false")
			}
			if opts.Alt != "responses/compact" {
				t.Fatalf("opts.Alt = %q, want %q", opts.Alt, "responses/compact")
			}
			if string(opts.OriginalRequest) != string(rawJSON) {
				t.Fatalf("opts.OriginalRequest = %s, want %s", string(opts.OriginalRequest), string(rawJSON))
			}
			if opts.SourceFormat != sdktranslator.FromString("openai") {
				t.Fatalf("opts.SourceFormat = %v, want %v", opts.SourceFormat, sdktranslator.FromString("openai"))
			}
			if got := opts.Metadata[coreexecutor.RequestedModelMetadataKey]; got != "gemini-2.5-pro" {
				t.Fatalf("requested model metadata = %v, want %q", got, "gemini-2.5-pro")
			}
		})
	}
}

func TestExecuteStreamWithAuthManagerAllowImageModel_PreservesExecutionModel(t *testing.T) {
	modelRegistry := registry.GetGlobalRegistry()
	const authID = "test-handlers-auth-manager-image-route-auth"
	modelRegistry.RegisterClient(authID, "openai", []*registry.ModelInfo{
		{ID: "image-alias", DisplayName: "gpt-image-2", Created: time.Now().Unix()},
	})
	t.Cleanup(func() {
		modelRegistry.UnregisterClient(authID)
	})

	manager := coreauth.NewManager(nil, nil, nil)
	executor := &authManagerStreamingTestExecutor{}
	manager.RegisterExecutor(executor)
	if _, err := manager.Register(context.Background(), &coreauth.Auth{
		ID:       authID,
		Provider: "openai",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	handler := NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager)
	rawJSON := []byte(`{"model":"gpt-5.4-mini","tools":[{"model":"gpt-image-2"}]}`)

	dataChan, _, errChan := handler.ExecuteStreamWithAuthManagerAllowImageModel(context.Background(), "openai-response", "gpt-5.4-mini", "image-alias", rawJSON, "")
	if dataChan == nil {
		t.Fatalf("expected data channel, got nil")
	}
	for range dataChan {
	}
	for range errChan {
	}

	if executor.lastStreamReq.Model != "gpt-5.4-mini" {
		t.Fatalf("stream request model = %q, want %q", executor.lastStreamReq.Model, "gpt-5.4-mini")
	}
	if got := executor.lastStreamOpts.Metadata[coreexecutor.RequestedModelMetadataKey]; got != "image-alias" {
		t.Fatalf("requested model metadata = %v, want %q", got, "image-alias")
	}
}
