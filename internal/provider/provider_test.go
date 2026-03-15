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

	p, eventType, err := Detect(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "github" {
		t.Errorf("got provider %q, want %q", p.Name(), "github")
	}
	if eventType != "push" {
		t.Errorf("got eventType %q, want %q", eventType, "push")
	}
}

func TestDetect_GitHub_Release(t *testing.T) {
	r := httptest.NewRequest("POST", "/webhook/test", nil)
	r.Header.Set("X-GitHub-Event", "release")

	p, eventType, err := Detect(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "github" {
		t.Errorf("got provider %q, want %q", p.Name(), "github")
	}
	if eventType != "release" {
		t.Errorf("got eventType %q, want %q", eventType, "release")
	}
}

func TestDetect_Generic(t *testing.T) {
	r := httptest.NewRequest("POST", "/webhook/test", nil)
	r.Header.Set("X-Deploq-Token", "test-token")

	p, eventType, err := Detect(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "generic" {
		t.Errorf("got provider %q, want %q", p.Name(), "generic")
	}
	if eventType != "push" {
		t.Errorf("got eventType %q, want %q", eventType, "push")
	}
}

func TestDetect_GitHubPriority(t *testing.T) {
	r := httptest.NewRequest("POST", "/webhook/test", nil)
	r.Header.Set("X-GitHub-Event", "push")
	r.Header.Set("X-Deploq-Token", "test-token")

	p, _, err := Detect(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "github" {
		t.Errorf("GitHub should have priority, got %q", p.Name())
	}
}

func TestDetect_Unknown(t *testing.T) {
	r := httptest.NewRequest("POST", "/webhook/test", nil)

	_, _, err := Detect(r)
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

func TestGitHub_ParseEvent_Push(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main","after":"abc123def456abc123def456abc123def456abc1"}`)

	g := &GitHub{}
	ev, err := g.ParseEvent(body, "push")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if ev.Branch != "main" {
		t.Errorf("Branch = %q, want %q", ev.Branch, "main")
	}
	if ev.SHA != "abc123def456abc123def456abc123def456abc1" {
		t.Errorf("SHA = %q, want %q", ev.SHA, "abc123def456abc123def456abc123def456abc1")
	}
	if ev.EventType != "push" {
		t.Errorf("EventType = %q, want %q", ev.EventType, "push")
	}
}

func TestGitHub_ParseEvent_Ping(t *testing.T) {
	g := &GitHub{}
	ev, err := g.ParseEvent([]byte(`{"zen":"test"}`), "ping")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.EventType != "ping" {
		t.Errorf("EventType = %q, want %q", ev.EventType, "ping")
	}
}

func TestGitHub_ParseEvent_Release(t *testing.T) {
	body := []byte(`{
		"action": "published",
		"release": {
			"tag_name": "v1.2.3",
			"target_commitish": "main"
		}
	}`)

	g := &GitHub{}
	ev, err := g.ParseEvent(body, "release")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if ev.EventType != "release" {
		t.Errorf("EventType = %q, want %q", ev.EventType, "release")
	}
	if ev.Ref != "refs/tags/v1.2.3" {
		t.Errorf("Ref = %q, want %q", ev.Ref, "refs/tags/v1.2.3")
	}
	if ev.Branch != "main" {
		t.Errorf("Branch = %q, want %q", ev.Branch, "main")
	}
	if ev.SHA != "" {
		t.Errorf("SHA should be empty for release events, got %q", ev.SHA)
	}
}

func TestGitHub_ParseEvent_Release_NotPublished(t *testing.T) {
	body := []byte(`{
		"action": "created",
		"release": {
			"tag_name": "v1.0.0",
			"target_commitish": "main"
		}
	}`)

	g := &GitHub{}
	_, err := g.ParseEvent(body, "release")
	if err == nil {
		t.Error("expected error for non-published release action")
	}
}

func TestGitHub_ParseEvent_Unsupported(t *testing.T) {
	g := &GitHub{}
	_, err := g.ParseEvent([]byte(`{}`), "issues")
	if err == nil {
		t.Error("expected error for unsupported event type")
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
	ev, err := g.ParseEvent(body, "push")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if ev.Branch != "develop" {
		t.Errorf("Branch = %q, want %q", ev.Branch, "develop")
	}
	if ev.SHA != "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef" {
		t.Errorf("SHA = %q, want %q", ev.SHA, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	}
	if ev.EventType != "push" {
		t.Errorf("EventType = %q, want %q", ev.EventType, "push")
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
			_, err := g.ParseEvent([]byte(tt.body), "push")
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
			_, err := g.ParseEvent([]byte(tt.body), "push")
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

func TestValidateTagName(t *testing.T) {
	valid := []string{"v1.0.0", "v1.0.0-rc.1", "release/2.0", "my_tag"}
	for _, tag := range valid {
		if err := ValidateTagName(tag); err != nil {
			t.Errorf("ValidateTagName(%q) unexpected error: %v", tag, err)
		}
	}

	invalid := []string{"", "../etc/passwd", "tag\nname", "a\x00b"}
	for _, tag := range invalid {
		if err := ValidateTagName(tag); err == nil {
			t.Errorf("ValidateTagName(%q) expected error", tag)
		}
	}
}

func TestValidateRefName(t *testing.T) {
	valid := []string{"main", "feature/my-branch", "release/1.0"}
	for _, ref := range valid {
		if err := ValidateRefName(ref); err != nil {
			t.Errorf("ValidateRefName(%q) unexpected error: %v", ref, err)
		}
	}

	invalid := []string{"", "main..feature", "ref\nname"}
	for _, ref := range invalid {
		if err := ValidateRefName(ref); err == nil {
			t.Errorf("ValidateRefName(%q) expected error", ref)
		}
	}
}

func TestGitHub_ParseEvent_Release_InvalidTagName(t *testing.T) {
	body := `{"action":"published","release":{"tag_name":"../evil","target_commitish":"main"}}`
	g := &GitHub{}
	_, err := g.ParseEvent([]byte(body), "release")
	if err == nil {
		t.Error("expected error for invalid tag_name")
	}
}

func TestGitHub_ParseEvent_Release_InvalidCommitish(t *testing.T) {
	body := `{"action":"published","release":{"tag_name":"v1.0.0","target_commitish":"main..evil"}}`
	g := &GitHub{}
	_, err := g.ParseEvent([]byte(body), "release")
	if err == nil {
		t.Error("expected error for invalid target_commitish")
	}
}
