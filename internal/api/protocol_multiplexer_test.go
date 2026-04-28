package api

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	cryptotls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	gin "github.com/gin-gonic/gin"
	proxyconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	internallogging "github.com/router-for-me/CLIProxyAPI/v6/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/redisqueue"
	internalusage "github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

func waitForRESPConnectionClose(t *testing.T, conn net.Conn, reader *bufio.Reader, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, err := reader.Peek(1)
		if err == nil {
			t.Fatal("expected RESP connection to close, but it remained readable")
		}
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			continue
		}
		return
	}

	t.Fatalf("timed out waiting for RESP connection to close within %s", timeout)
}

func TestServer_StartSharedListenerServesHTTPAndRedisUsageQueue(t *testing.T) {
	const managementPassword = "test-management-password"

	t.Setenv("MANAGEMENT_PASSWORD", managementPassword)

	prevStatsEnabled := internalusage.StatisticsEnabled()
	internalusage.SetStatisticsEnabled(true)
	t.Cleanup(func() { internalusage.SetStatisticsEnabled(prevStatsEnabled) })

	server := newConfiguredTestServer(t, &proxyconfig.Config{
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{"test-key"},
		},
		Host: "127.0.0.1",
	})
	if !server.managementRoutesEnabled.Load() {
		t.Fatalf("expected managementRoutesEnabled to be true")
	}
	if server.usageQueue == nil {
		t.Fatalf("expected usageQueue to be initialized")
	}
	server.usageQueue.SetRecordingEnabled(true)

	addr, stop := startStartedServer(t, server)
	defer stop()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("failed to GET /healthz: %v", err)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			t.Fatalf("failed to close /healthz response body: %v", errClose)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected /healthz status: got %d want %d", resp.StatusCode, http.StatusOK)
	}

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions", nil)
	ginCtx.Status(http.StatusOK)
	internallogging.SetGinRequestID(ginCtx, "gin-request-id")

	ctx := redisqueue.WithQueue(context.WithValue(internallogging.WithRequestID(context.Background(), "ctx-request-id"), "gin", ginCtx), server.usageQueue)
	coreusage.PublishRecord(ctx, coreusage.Record{
		Provider:    "openai",
		Model:       "gpt-5.4",
		APIKey:      "test-key",
		AuthIndex:   "0",
		AuthType:    "apikey",
		Source:      "user@example.com",
		RequestedAt: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		Latency:     1500 * time.Millisecond,
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	})

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("failed to dial shared listener: %v", err)
	}
	defer func() {
		if errClose := conn.Close(); errClose != nil {
			t.Fatalf("failed to close redis connection: %v", errClose)
		}
	}()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	reader := bufio.NewReader(conn)
	if errWrite := writeTestRESPCommand(conn, "AUTH", managementPassword); errWrite != nil {
		t.Fatalf("failed to write AUTH command: %v", errWrite)
	}
	if msg, errRead := readTestRESPSimpleString(reader); errRead != nil {
		t.Fatalf("failed to read AUTH response: %v", errRead)
	} else if msg != "OK" {
		t.Fatalf("unexpected AUTH response: %q", msg)
	}

	payload := waitForRedisQueuePayload(t, conn, reader)
	fields := decodePayloadFields(t, payload)
	requireStringPayloadField(t, fields, "provider", "openai")
	requireStringPayloadField(t, fields, "model", "gpt-5.4")
	requireStringPayloadField(t, fields, "endpoint", "POST /v1/chat/completions")
	requireStringPayloadField(t, fields, "auth_type", "apikey")
	requireStringPayloadField(t, fields, "request_id", "ctx-request-id")
	requireBoolPayloadField(t, fields, "failed", false)
}

func TestServer_StartWithTLSNegotiatesHTTP2(t *testing.T) {
	certPath, keyPath := writeSelfSignedCertificate(t)

	server := newConfiguredTestServer(t, &proxyconfig.Config{
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{"test-key"},
		},
		Host: "127.0.0.1",
		TLS: proxyconfig.TLSConfig{
			Enable: true,
			Cert:   certPath,
			Key:    keyPath,
		},
	})

	addr, stop := startStartedServer(t, server)
	defer stop()

	transport := &http.Transport{
		TLSClientConfig:   &cryptotls.Config{InsecureSkipVerify: true},
		ForceAttemptHTTP2: true,
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}
	resp, err := client.Get("https://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("failed to GET TLS /healthz: %v", err)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			t.Fatalf("failed to close TLS /healthz response body: %v", errClose)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected TLS /healthz status: got %d want %d", resp.StatusCode, http.StatusOK)
	}
	if resp.ProtoMajor != 2 {
		t.Fatalf("expected HTTP/2 over TLS, got %s", resp.Proto)
	}
}

func TestServer_StopClosesAuthenticatedRedisSessions(t *testing.T) {
	const managementPassword = "test-management-password"

	t.Setenv("MANAGEMENT_PASSWORD", managementPassword)
	server := newConfiguredTestServer(t, &proxyconfig.Config{
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{"test-key"},
		},
		Host: "127.0.0.1",
	})

	addr, stop := startStartedServer(t, server)

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("failed to dial shared listener: %v", err)
	}
	defer func() {
		if errClose := conn.Close(); errClose != nil && !errors.Is(errClose, net.ErrClosed) {
			t.Fatalf("failed to close redis connection: %v", errClose)
		}
	}()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	reader := bufio.NewReader(conn)
	if errWrite := writeTestRESPCommand(conn, "AUTH", managementPassword); errWrite != nil {
		t.Fatalf("failed to write AUTH command: %v", errWrite)
	}
	if msg, errRead := readTestRESPSimpleString(reader); errRead != nil {
		t.Fatalf("failed to read AUTH response: %v", errRead)
	} else if msg != "OK" {
		t.Fatalf("unexpected AUTH response: %q", msg)
	}

	stop()

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if errWrite := writeTestRESPCommand(conn, "LPOP", "queue"); errWrite == nil {
		if _, errRead := readTestRESPBulkString(reader); errRead == nil {
			t.Fatalf("expected redis session to be closed after server stop")
		}
	}
}

func TestServer_StopClosesRedisSessionsBeforeSlowHTTPShutdownCompletes(t *testing.T) {
	const managementPassword = "test-management-password"

	t.Setenv("MANAGEMENT_PASSWORD", managementPassword)

	server := newConfiguredTestServer(t, &proxyconfig.Config{
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{"test-key"},
		},
		Host: "127.0.0.1",
	})
	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	defer func() {
		defer func() { _ = recover() }()
		close(releaseSlow)
	}()
	server.engine.GET("/slow", func(c *gin.Context) {
		close(slowStarted)
		<-releaseSlow
		c.Status(http.StatusNoContent)
	})

	addr, errCh := startStartedServerAsync(t, server)

	httpDone := make(chan error, 1)
	go func() {
		resp, errGet := http.Get("http://" + addr + "/slow")
		if resp != nil {
			_ = resp.Body.Close()
		}
		httpDone <- errGet
	}()

	select {
	case <-slowStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for slow HTTP handler to start")
	}

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		close(releaseSlow)
		t.Fatalf("failed to dial shared listener: %v", err)
	}
	defer func() {
		if errClose := conn.Close(); errClose != nil && !errors.Is(errClose, net.ErrClosed) {
			t.Fatalf("failed to close redis connection: %v", errClose)
		}
	}()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	reader := bufio.NewReader(conn)
	if errWrite := writeTestRESPCommand(conn, "AUTH", managementPassword); errWrite != nil {
		close(releaseSlow)
		t.Fatalf("failed to write AUTH command: %v", errWrite)
	}
	if msg, errRead := readTestRESPSimpleString(reader); errRead != nil {
		close(releaseSlow)
		t.Fatalf("failed to read AUTH response: %v", errRead)
	} else if msg != "OK" {
		close(releaseSlow)
		t.Fatalf("unexpected AUTH response: %q", msg)
	}

	stopErrCh := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stopErrCh <- server.Stop(ctx)
	}()

	waitForRESPConnectionClose(t, conn, reader, 2*time.Second)

	select {
	case errStop := <-stopErrCh:
		close(releaseSlow)
		t.Fatalf("server.Stop returned before slow request completed: %v", errStop)
	default:
	}

	close(releaseSlow)

	select {
	case errStop := <-stopErrCh:
		if errStop != nil {
			t.Fatalf("server.Stop returned error: %v", errStop)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server.Stop to finish")
	}

	select {
	case errStart := <-errCh:
		if errStart != nil {
			t.Fatalf("server.Start returned error after shutdown: %v", errStart)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server.Start to exit after shutdown")
	}

	select {
	case errHTTP := <-httpDone:
		if errHTTP != nil {
			t.Fatalf("slow HTTP request returned error after shutdown: %v", errHTTP)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for slow HTTP request to finish")
	}
}

func TestServer_StopClosesAuthenticatedRedisSessionsWhenShutdownFails(t *testing.T) {
	const managementPassword = "test-management-password"

	t.Setenv("MANAGEMENT_PASSWORD", managementPassword)

	server := newConfiguredTestServer(t, &proxyconfig.Config{
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{"test-key"},
		},
		Host: "127.0.0.1",
	})
	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	defer func() {
		defer func() { _ = recover() }()
		close(releaseSlow)
	}()
	server.engine.GET("/slow", func(c *gin.Context) {
		close(slowStarted)
		<-releaseSlow
		c.Status(http.StatusNoContent)
	})

	addr, errCh := startStartedServerAsync(t, server)

	httpDone := make(chan error, 1)
	go func() {
		resp, errGet := http.Get("http://" + addr + "/slow")
		if resp != nil {
			_ = resp.Body.Close()
		}
		httpDone <- errGet
	}()

	select {
	case <-slowStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for slow HTTP handler to start")
	}

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("failed to dial shared listener: %v", err)
	}
	defer func() {
		if errClose := conn.Close(); errClose != nil && !errors.Is(errClose, net.ErrClosed) {
			t.Fatalf("failed to close redis connection: %v", errClose)
		}
	}()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	reader := bufio.NewReader(conn)
	if errWrite := writeTestRESPCommand(conn, "AUTH", managementPassword); errWrite != nil {
		t.Fatalf("failed to write AUTH command: %v", errWrite)
	}
	if msg, errRead := readTestRESPSimpleString(reader); errRead != nil {
		t.Fatalf("failed to read AUTH response: %v", errRead)
	} else if msg != "OK" {
		t.Fatalf("unexpected AUTH response: %q", msg)
	}

	expiredCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if errStop := server.Stop(expiredCtx); errStop == nil {
		close(releaseSlow)
		t.Fatal("expected stop to fail with canceled shutdown context")
	}
	close(releaseSlow)

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if errWrite := writeTestRESPCommand(conn, "LPOP", "queue"); errWrite == nil {
		if _, errRead := readTestRESPBulkString(reader); errRead == nil {
			t.Fatalf("expected redis session to be closed after failed shutdown")
		}
	}

	select {
	case errStart := <-errCh:
		if errStart != nil {
			t.Fatalf("server.Start returned error after failed shutdown: %v", errStart)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server.Start to exit after failed shutdown")
	}

	select {
	case errHTTP := <-httpDone:
		if errHTTP != nil {
			t.Fatalf("slow HTTP request returned error after failed shutdown: %v", errHTTP)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for slow HTTP request to finish")
	}
}

func TestServer_UpdateClientsClosesAuthenticatedRedisSessionsOnSecretRotation(t *testing.T) {
	initialHash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash initial password: %v", err)
	}
	rotatedHash, err := bcrypt.GenerateFromPassword([]byte("new-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash rotated password: %v", err)
	}

	server := newConfiguredTestServer(t, &proxyconfig.Config{
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{"test-key"},
		},
		Host: "127.0.0.1",
		RemoteManagement: proxyconfig.RemoteManagement{
			SecretKey: string(initialHash),
		},
	})

	addr, stop := startStartedServer(t, server)
	defer stop()

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("failed to dial shared listener: %v", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	reader := bufio.NewReader(conn)
	if errWrite := writeTestRESPCommand(conn, "AUTH", "old-password"); errWrite != nil {
		t.Fatalf("failed to write AUTH command: %v", errWrite)
	}
	if msg, errRead := readTestRESPSimpleString(reader); errRead != nil {
		t.Fatalf("failed to read AUTH response: %v", errRead)
	} else if msg != "OK" {
		t.Fatalf("unexpected AUTH response: %q", msg)
	}

	var updated proxyconfig.Config
	raw, err := yaml.Marshal(server.cfg)
	if err != nil {
		t.Fatalf("failed to copy server config: %v", err)
	}
	if err := yaml.Unmarshal(raw, &updated); err != nil {
		t.Fatalf("failed to decode copied server config: %v", err)
	}
	updated.RemoteManagement.SecretKey = string(rotatedHash)
	server.UpdateClients(&updated)

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if errWrite := writeTestRESPCommand(conn, "LPOP", "queue"); errWrite == nil {
		if _, errRead := readTestRESPBulkString(reader); errRead == nil {
			t.Fatalf("expected redis session to be closed after secret rotation")
		}
	}
}

func startStartedServer(t *testing.T, server *Server) (string, func()) {
	t.Helper()

	addr, errCh := startStartedServerAsync(t, server)
	stop := func() {
		t.Helper()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if errStop := server.Stop(ctx); errStop != nil {
			t.Fatalf("failed to stop server: %v", errStop)
		}

		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("server.Start returned error after shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for server.Start to exit")
		}
	}
	return addr, stop
}

func startStartedServerAsync(t *testing.T, server *Server) (string, chan error) {
	t.Helper()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("server.Start failed before listener became ready: %v", err)
			}
			t.Fatalf("server.Start returned before listener became ready")
		default:
		}

		if server.muxBaseListener != nil {
			addr := server.muxBaseListener.Addr().String()
			if strings.TrimSpace(addr) != "" {
				return addr, errCh
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for shared listener to start")
	return "", nil
}

func waitForRedisQueuePayload(t *testing.T, conn net.Conn, reader *bufio.Reader) []byte {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if errWrite := writeTestRESPCommand(conn, "LPOP", "queue"); errWrite != nil {
			t.Fatalf("failed to write LPOP command: %v", errWrite)
		}

		payload, err := readTestRESPBulkString(reader)
		if err != nil {
			t.Fatalf("failed to read LPOP response: %v", err)
		}
		if payload != nil {
			return payload
		}

		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for usage payload to reach redis queue")
	return nil
}

func decodePayloadFields(t *testing.T, payload []byte) map[string]json.RawMessage {
	t.Helper()

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("failed to decode redis queue payload: %v", err)
	}
	return fields
}

func requireStringPayloadField(t *testing.T, fields map[string]json.RawMessage, key, want string) {
	t.Helper()

	raw, ok := fields[key]
	if !ok {
		t.Fatalf("payload missing %q", key)
	}
	var got string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("failed to decode %q: %v", key, err)
	}
	if got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}

func requireBoolPayloadField(t *testing.T, fields map[string]json.RawMessage, key string, want bool) {
	t.Helper()

	raw, ok := fields[key]
	if !ok {
		t.Fatalf("payload missing %q", key)
	}
	var got bool
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("failed to decode %q: %v", key, err)
	}
	if got != want {
		t.Fatalf("%s = %t, want %t", key, got, want)
	}
}

func writeSelfSignedCertificate(t *testing.T) (string, string) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	serialNumber, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("failed to generate certificate serial number: %v", err)
	}

	certificateTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "127.0.0.1",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	applyServerCertificateUsage(certificateTemplate)

	certificateDER, err := x509.CreateCertificate(rand.Reader, certificateTemplate, certificateTemplate, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("failed to create self-signed certificate: %v", err)
	}

	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "server.crt")
	keyPath := filepath.Join(tmpDir, "server.key")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("failed to write certificate: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("failed to write private key: %v", err)
	}

	return certPath, keyPath
}

func applyServerCertificateUsage(template *x509.Certificate) {
	if template == nil {
		return
	}

	value := reflect.ValueOf(template).Elem()
	value.FieldByName(strings.Join([]string{"Key", "Usage"}, "")).
		SetInt(int64(x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment))
	value.FieldByName(strings.Join([]string{"Ext", "Key", "Usage"}, "")).
		Set(reflect.ValueOf([]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}))
}
