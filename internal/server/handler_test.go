package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/uscompany/pushup/internal/config"
	"github.com/uscompany/pushup/internal/deploy"
)

func testConfig() *config.Config {
	return &config.Config{
		Listen: ":9090",
		Projects: map[string]*config.ProjectConfig{
			"test-app": {
				Path:          "/tmp/test-app",
				Branch:        "main",
				Secret:        "test-secret-long-enough-value",
				ComposeFile:   "docker-compose.yml",
				DeployTimeout: 15 * time.Minute,
			},
		},
	}
}

func signBody(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestHandleHealth(t *testing.T) {
	cfg := testConfig()
	deployer := deploy.New(cfg)
	srv := New(cfg, deployer)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
}

func TestHandleWebhook_InvalidProjectName(t *testing.T) {
	cfg := testConfig()
	deployer := deploy.New(cfg)
	srv := New(cfg, deployer)

	body := []byte(`{"ref":"refs/heads/main","sha":"abc1234abc1234abc1234abc1234abc1234abc12"}`)
	req := httptest.NewRequest("POST", "/webhook/bad%00name", bytes.NewReader(body))
	req.Header.Set("X-Pushup-Token", "some-token")
	req.SetPathValue("project", "bad\x00name")
	w := httptest.NewRecorder()
	srv.handleWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleWebhook_UnknownProject(t *testing.T) {
	cfg := testConfig()
	deployer := deploy.New(cfg)
	srv := New(cfg, deployer)

	body := []byte(`{"ref":"refs/heads/main","sha":"abc1234abc1234abc1234abc1234abc1234abc12"}`)
	req := httptest.NewRequest("POST", "/webhook/nonexistent", bytes.NewReader(body))
	req.Header.Set("X-Pushup-Token", "some-token")
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleWebhook_NoProvider(t *testing.T) {
	cfg := testConfig()
	deployer := deploy.New(cfg)
	srv := New(cfg, deployer)

	body := []byte(`{"ref":"refs/heads/main","sha":"abc1234abc1234abc1234abc1234abc1234abc12"}`)
	req := httptest.NewRequest("POST", "/webhook/test-app", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleWebhook_InvalidToken(t *testing.T) {
	cfg := testConfig()
	deployer := deploy.New(cfg)
	srv := New(cfg, deployer)

	body := []byte(`{"ref":"refs/heads/main","sha":"abc1234abc1234abc1234abc1234abc1234abc12"}`)
	req := httptest.NewRequest("POST", "/webhook/test-app", bytes.NewReader(body))
	req.Header.Set("X-Pushup-Token", "wrong-token")
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleWebhook_BranchMismatch(t *testing.T) {
	cfg := testConfig()
	deployer := deploy.New(cfg)
	srv := New(cfg, deployer)

	secret := cfg.Projects["test-app"].Secret
	body := []byte(`{"ref":"refs/heads/develop","sha":"abc1234abc1234abc1234abc1234abc1234abc12"}`)

	req := httptest.NewRequest("POST", "/webhook/test-app", bytes.NewReader(body))
	req.Header.Set("X-Pushup-Token", secret)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "skipped" {
		t.Errorf("status = %q, want %q", resp["status"], "skipped")
	}
}

func TestHandleWebhook_GitHub_ValidSignature(t *testing.T) {
	cfg := testConfig()
	deployer := deploy.New(cfg)
	srv := New(cfg, deployer)

	secret := cfg.Projects["test-app"].Secret
	body := []byte(`{"ref":"refs/heads/main","after":"abc1234abc1234abc1234abc1234abc1234abc12"}`)
	sig := signBody(body, secret)

	req := httptest.NewRequest("POST", "/webhook/test-app", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", sig)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "accepted" {
		t.Errorf("status = %q, want %q", resp["status"], "accepted")
	}
}

func TestHandleWebhook_Generic_ValidToken(t *testing.T) {
	cfg := testConfig()
	deployer := deploy.New(cfg)
	srv := New(cfg, deployer)

	secret := cfg.Projects["test-app"].Secret
	body := []byte(`{"ref":"refs/heads/main","sha":"def456def456def456def456def456def456def4"}`)

	req := httptest.NewRequest("POST", "/webhook/test-app", bytes.NewReader(body))
	req.Header.Set("X-Pushup-Token", secret)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestHandleWebhook_FailedDeploy_AllowsRetry(t *testing.T) {
	cfg := testConfig()
	deployer := deploy.New(cfg)
	srv := New(cfg, deployer)

	secret := cfg.Projects["test-app"].Secret
	body := []byte(`{"ref":"refs/heads/main","sha":"aabbcc1122aabbcc1122aabbcc1122aabbcc1122"}`)

	// First request — accepted (deploy will fail since /tmp/test-app doesn't exist)
	req1 := httptest.NewRequest("POST", "/webhook/test-app", bytes.NewReader(body))
	req1.Header.Set("X-Pushup-Token", secret)
	w1 := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w1, req1)

	if w1.Code != http.StatusAccepted {
		t.Fatalf("first request status = %d, want %d", w1.Code, http.StatusAccepted)
	}

	deployer.Wait()

	// Second request with same SHA — should be accepted again (failed deploys don't record SHA)
	req2 := httptest.NewRequest("POST", "/webhook/test-app", bytes.NewReader(body))
	req2.Header.Set("X-Pushup-Token", secret)
	w2 := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusAccepted {
		t.Errorf("second request status = %d, want %d (retry after failure)", w2.Code, http.StatusAccepted)
	}
}
