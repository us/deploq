package provider

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDetect_GitHub(t *testing.T) {
	r := httptest.NewRequest("POST", "/webhook/test", nil)
	r.Header.Set("X-GitHub-Event", "push")

	p, err := Detect(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "github" {
		t.Errorf("got provider %q, want %q", p.Name(), "github")
	}
}

func TestDetect_Generic(t *testing.T) {
	r := httptest.NewRequest("POST", "/webhook/test", nil)
	r.Header.Set("X-Deploq-Token", "test-token")

	p, err := Detect(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "generic" {
		t.Errorf("got provider %q, want %q", p.Name(), "generic")
	}
}

func TestDetect_GitHubPriority(t *testing.T) {
	r := httptest.NewRequest("POST", "/webhook/test", nil)
	r.Header.Set("X-GitHub-Event", "push")
	r.Header.Set("X-Deploq-Token", "test-token")

	p, err := Detect(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "github" {
		t.Errorf("GitHub should have priority, got %q", p.Name())
	}
}

func TestDetect_Unknown(t *testing.T) {
	r := httptest.NewRequest("POST", "/webhook/test", nil)

	_, err := Detect(r)
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestGitHub_Verify_Valid(t *testing.T) {
	secret := "test-secret-long-enough"
	body := []byte(`{"ref":"refs/heads/main","after":"abc123"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("X-Hub-Signature-256", sig)

	g := &GitHub{}
	if err := g.Verify(r, body, secret); err != nil {
		t.Errorf("verify failed: %v", err)
	}
}

func TestGitHub_Verify_Invalid(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")

	g := &GitHub{}
	if err := g.Verify(r, []byte("body"), "secret"); err == nil {
		t.Error("expected error for invalid signature")
	}
}

func TestGitHub_Verify_MissingHeader(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)

	g := &GitHub{}
	if err := g.Verify(r, []byte("body"), "secret"); err == nil {
		t.Error("expected error for missing header")
	}
}

func TestGitHub_ParseEvent(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main","after":"abc123def456abc123def456abc123def456abc1"}`)

	g := &GitHub{}
	ev, err := g.ParseEvent(body)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if ev.Branch != "main" {
		t.Errorf("Branch = %q, want %q", ev.Branch, "main")
	}
	if ev.SHA != "abc123def456abc123def456abc123def456abc1" {
		t.Errorf("SHA = %q, want %q", ev.SHA, "abc123def456abc123def456abc123def456abc1")
	}
}

func TestGeneric_Verify_Valid(t *testing.T) {
	secret := "my-secret-token"
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("X-Deploq-Token", secret)

	g := &Generic{}
	if err := g.Verify(r, nil, secret); err != nil {
		t.Errorf("verify failed: %v", err)
	}
}

func TestGeneric_Verify_Invalid(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("X-Deploq-Token", "wrong-token")

	g := &Generic{}
	if err := g.Verify(r, nil, "correct-token"); err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestGeneric_ParseEvent(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/develop","sha":"deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"}`)

	g := &Generic{}
	ev, err := g.ParseEvent(body)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if ev.Branch != "develop" {
		t.Errorf("Branch = %q, want %q", ev.Branch, "develop")
	}
	if ev.SHA != "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef" {
		t.Errorf("SHA = %q, want %q", ev.SHA, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	}
}

func TestGeneric_ParseEvent_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"missing ref", `{"sha":"abc"}`},
		{"missing sha", `{"ref":"refs/heads/main"}`},
		{"empty body", `{}`},
	}

	g := &Generic{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := g.ParseEvent([]byte(tt.body))
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestGitHub_ParseEvent_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"missing ref", `{"after":"abc"}`},
		{"missing after", `{"ref":"refs/heads/main"}`},
	}

	g := &GitHub{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := g.ParseEvent([]byte(tt.body))
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestGitHub_Verify_EmptySecret(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")

	g := &GitHub{}
	if err := g.Verify(r, []byte("body"), ""); err == nil {
		t.Error("expected error for empty secret")
	}
}

func TestGeneric_Verify_EmptySecret(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("X-Deploq-Token", "some-token")

	g := &Generic{}
	if err := g.Verify(r, nil, ""); err == nil {
		t.Error("expected error for empty secret")
	}
}

func TestValidateSHA_TooShort(t *testing.T) {
	if err := ValidateSHA("abc12"); err == nil {
		t.Error("expected error for 5-char SHA (below minimum 7)")
	}
}

func TestValidateSHA_Valid40(t *testing.T) {
	if err := ValidateSHA("abc1234abc1234abc1234abc1234abc1234abc12"); err != nil {
		t.Errorf("unexpected error for valid 40-char SHA: %v", err)
	}
}

func TestGitHub_Verify_InvalidHex(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader("body"))
	r.Header.Set("X-Hub-Signature-256", "sha256=not-valid-hex!")

	g := &GitHub{}
	if err := g.Verify(r, []byte("body"), "secret"); err == nil {
		t.Error("expected error for invalid hex")
	}
}
