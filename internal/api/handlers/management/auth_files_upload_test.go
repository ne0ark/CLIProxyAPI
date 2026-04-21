package management

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

type uploadMode string

const (
	uploadModeMultipart uploadMode = "multipart"
	uploadModeRawBody   uploadMode = "raw"
)

type comparableRuntimeAuth struct {
	ID          string
	Provider    string
	Label       string
	FileName    string
	Prefix      string
	ProxyURL    string
	Disabled    bool
	Status      coreauth.Status
	Priority    string
	Note        string
	HeaderAuth  string
	HeaderTrace string
}

func TestUploadAuthFile_PropagatesRuntimeFieldsImmediately(t *testing.T) {
	content := `{
		"type":"codex",
		"email":"alpha@example.com",
		"priority":" 98 ",
		"prefix":" /team-a/ ",
		"proxy_url":"http://proxy.local:8080",
		"disabled":true,
		"note":"  production key  ",
		"headers":{
			"Authorization":" Bearer token ",
			"X-Trace":" trace-id "
		}
	}`

	h, manager := newUploadTestHandler(t)
	uploadAuthFile(t, h, uploadModeMultipart, "alpha.json", content)

	auth := mustGetUploadedAuth(t, manager, "alpha.json")
	if auth.Provider != "codex" {
		t.Fatalf("provider = %q, want %q", auth.Provider, "codex")
	}
	if auth.Label != "alpha@example.com" {
		t.Fatalf("label = %q, want %q", auth.Label, "alpha@example.com")
	}
	if auth.FileName != "alpha.json" {
		t.Fatalf("file name = %q, want %q", auth.FileName, "alpha.json")
	}
	if auth.Prefix != "team-a" {
		t.Fatalf("prefix = %q, want %q", auth.Prefix, "team-a")
	}
	if auth.ProxyURL != "http://proxy.local:8080" {
		t.Fatalf("proxy_url = %q, want %q", auth.ProxyURL, "http://proxy.local:8080")
	}
	if !auth.Disabled {
		t.Fatal("expected uploaded auth to be disabled immediately")
	}
	if auth.Status != coreauth.StatusDisabled {
		t.Fatalf("status = %q, want %q", auth.Status, coreauth.StatusDisabled)
	}
	if got := auth.Attributes["priority"]; got != "98" {
		t.Fatalf("priority attribute = %q, want %q", got, "98")
	}
	if got := auth.Attributes["note"]; got != "production key" {
		t.Fatalf("note attribute = %q, want %q", got, "production key")
	}
	if got := auth.Attributes["header:Authorization"]; got != "Bearer token" {
		t.Fatalf("header Authorization = %q, want %q", got, "Bearer token")
	}
	if got := auth.Attributes["header:X-Trace"]; got != "trace-id" {
		t.Fatalf("header X-Trace = %q, want %q", got, "trace-id")
	}
	if got, ok := auth.Metadata["priority"].(string); !ok || got != " 98 " {
		t.Fatalf("metadata priority = %#v, want raw string %q", auth.Metadata["priority"], " 98 ")
	}
}

func TestUploadAuthFile_MultipartAndRawBodyProduceEquivalentRuntimeState(t *testing.T) {
	content := `{
		"type":"codex",
		"email":"shared@example.com",
		"priority":12,
		"prefix":" /shared/ ",
		"proxy_url":"http://proxy.shared:9000",
		"note":"  shared note  ",
		"headers":{
			"Authorization":" Bearer shared ",
			"X-Trace":" shared-trace "
		}
	}`

	multipartHandler, multipartManager := newUploadTestHandler(t)
	uploadAuthFile(t, multipartHandler, uploadModeMultipart, "shared-multipart.json", content)
	multipartAuth := mustGetUploadedAuth(t, multipartManager, "shared-multipart.json")

	rawHandler, rawManager := newUploadTestHandler(t)
	uploadAuthFile(t, rawHandler, uploadModeRawBody, "shared-raw.json", content)
	rawAuth := mustGetUploadedAuth(t, rawManager, "shared-raw.json")

	multipartSnapshot := comparableRuntimeState(multipartAuth)
	rawSnapshot := comparableRuntimeState(rawAuth)
	multipartSnapshot.ID = ""
	multipartSnapshot.FileName = ""
	rawSnapshot.ID = ""
	rawSnapshot.FileName = ""
	if !reflect.DeepEqual(multipartSnapshot, rawSnapshot) {
		t.Fatalf("multipart snapshot %#v did not match raw snapshot %#v", multipartSnapshot, rawSnapshot)
	}
}

func TestUploadAuthFile_PriorityAffectsSelectionImmediately(t *testing.T) {
	h, manager := newUploadTestHandler(t)
	if _, err := manager.Register(context.Background(), &coreauth.Auth{
		ID:       "low-priority.json",
		FileName: "low-priority.json",
		Provider: "codex",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"path":     "low-priority.json",
			"source":   "low-priority.json",
			"priority": "1",
		},
	}); err != nil {
		t.Fatalf("failed to register baseline auth: %v", err)
	}

	uploadAuthFile(t, h, uploadModeRawBody, "high-priority.json", `{"type":"codex","email":"high@example.com","priority":9}`)

	selected, err := (&coreauth.FillFirstSelector{}).Pick(
		context.Background(),
		"codex",
		"",
		cliproxyexecutor.Options{},
		manager.List(),
	)
	if err != nil {
		t.Fatalf("selector pick failed: %v", err)
	}
	if selected == nil {
		t.Fatal("expected selector to choose an auth")
	}
	if selected.ID != "high-priority.json" {
		t.Fatalf("selected auth = %q, want %q", selected.ID, "high-priority.json")
	}
}

func TestUploadAuthFile_ReuploadPreservesIdentityAndTimestampsWhileUpdatingRuntimeFields(t *testing.T) {
	h, manager := newUploadTestHandler(t)
	uploadAuthFile(t, h, uploadModeRawBody, "reupload.json", `{
		"type":"codex",
		"email":"reupload@example.com",
		"priority":1,
		"prefix":" /old/ ",
		"proxy_url":"http://old.proxy",
		"note":" first "
	}`)

	existing := mustGetUploadedAuth(t, manager, "reupload.json")
	createdAt := time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)
	lastRefresh := time.Date(2024, time.January, 3, 4, 5, 6, 0, time.UTC)
	existing.CreatedAt = createdAt
	existing.LastRefreshedAt = lastRefresh
	existing.Runtime = "runtime-sentinel"
	existing.UpdatedAt = createdAt
	if _, err := manager.Update(context.Background(), existing); err != nil {
		t.Fatalf("failed to seed existing auth timestamps: %v", err)
	}

	uploadAuthFile(t, h, uploadModeMultipart, "reupload.json", `{
		"type":"codex",
		"email":"reupload@example.com",
		"priority":11,
		"prefix":" /new/ ",
		"proxy_url":"http://new.proxy",
		"note":" second "
	}`)

	updated := mustGetUploadedAuth(t, manager, "reupload.json")
	if updated.ID != existing.ID {
		t.Fatalf("id = %q, want preserved %q", updated.ID, existing.ID)
	}
	if !updated.CreatedAt.Equal(createdAt) {
		t.Fatalf("created_at = %s, want %s", updated.CreatedAt, createdAt)
	}
	if !updated.LastRefreshedAt.Equal(lastRefresh) {
		t.Fatalf("last_refreshed_at = %s, want %s", updated.LastRefreshedAt, lastRefresh)
	}
	if updated.Runtime != "runtime-sentinel" {
		t.Fatalf("runtime = %#v, want sentinel", updated.Runtime)
	}
	if got := updated.Attributes["priority"]; got != "11" {
		t.Fatalf("priority attribute = %q, want %q", got, "11")
	}
	if got := updated.Attributes["note"]; got != "second" {
		t.Fatalf("note attribute = %q, want %q", got, "second")
	}
	if updated.Prefix != "new" {
		t.Fatalf("prefix = %q, want %q", updated.Prefix, "new")
	}
	if updated.ProxyURL != "http://new.proxy" {
		t.Fatalf("proxy_url = %q, want %q", updated.ProxyURL, "http://new.proxy")
	}
	if !updated.UpdatedAt.After(createdAt) {
		t.Fatalf("updated_at = %s, want after preserved created_at %s", updated.UpdatedAt, createdAt)
	}
}

func newUploadTestHandler(t *testing.T) (*Handler, *coreauth.Manager) {
	t.Helper()
	t.Setenv("MANAGEMENT_PASSWORD", "")
	setGinTestMode()

	manager := coreauth.NewManager(nil, nil, nil)
	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, manager)
	return h, manager
}

func uploadAuthFile(t *testing.T, h *Handler, mode uploadMode, name string, content string) {
	t.Helper()

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	switch mode {
	case uploadModeMultipart:
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", name)
		if err != nil {
			t.Fatalf("failed to create multipart file: %v", err)
		}
		if _, err = part.Write([]byte(content)); err != nil {
			t.Fatalf("failed to write multipart content: %v", err)
		}
		if err = writer.Close(); err != nil {
			t.Fatalf("failed to close multipart writer: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/v0/management/auth-files", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		ctx.Request = req
	case uploadModeRawBody:
		req := httptest.NewRequest(
			http.MethodPost,
			"/v0/management/auth-files?name="+name,
			strings.NewReader(content),
		)
		req.Header.Set("Content-Type", "application/json")
		ctx.Request = req
	default:
		t.Fatalf("unsupported upload mode %q", mode)
	}

	h.UploadAuthFile(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected upload status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response payload: %v", err)
	}
	if status, _ := payload["status"].(string); status != "ok" {
		t.Fatalf("expected upload status payload %q, got %#v", "ok", payload["status"])
	}
}

func mustGetUploadedAuth(t *testing.T, manager *coreauth.Manager, id string) *coreauth.Auth {
	t.Helper()

	auth, ok := manager.GetByID(id)
	if !ok || auth == nil {
		t.Fatalf("expected auth %q to exist", id)
	}
	return auth
}

func comparableRuntimeState(auth *coreauth.Auth) comparableRuntimeAuth {
	if auth == nil {
		return comparableRuntimeAuth{}
	}
	return comparableRuntimeAuth{
		ID:          auth.ID,
		Provider:    auth.Provider,
		Label:       auth.Label,
		FileName:    auth.FileName,
		Prefix:      auth.Prefix,
		ProxyURL:    auth.ProxyURL,
		Disabled:    auth.Disabled,
		Status:      auth.Status,
		Priority:    auth.Attributes["priority"],
		Note:        auth.Attributes["note"],
		HeaderAuth:  auth.Attributes["header:Authorization"],
		HeaderTrace: auth.Attributes["header:X-Trace"],
	}
}
