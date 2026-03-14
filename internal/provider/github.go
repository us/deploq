package provider

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// GitHub implements the Provider interface for GitHub webhooks.
type GitHub struct{}

func (g *GitHub) Name() string { return "github" }

// Verify checks the X-Hub-Signature-256 header using HMAC-SHA256.
// Uses hmac.Equal for constant-time comparison (prevents timing attacks).
func (g *GitHub) Verify(r *http.Request, body []byte, secret string) error {
	if secret == "" {
		return fmt.Errorf("webhook secret is not configured")
	}

	sigHeader := r.Header.Get("X-Hub-Signature-256")
	if sigHeader == "" {
		return fmt.Errorf("missing X-Hub-Signature-256 header")
	}

	if !strings.HasPrefix(sigHeader, "sha256=") {
		return fmt.Errorf("invalid signature format: expected sha256= prefix")
	}
	receivedSig, err := hex.DecodeString(strings.TrimPrefix(sigHeader, "sha256="))
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedSig := mac.Sum(nil)

	if !hmac.Equal(receivedSig, expectedSig) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

// ParseEvent extracts ref and SHA from a GitHub push event payload.
func (g *GitHub) ParseEvent(body []byte) (Event, error) {
	var payload struct {
		Ref   string `json:"ref"`
		After string `json:"after"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Event{}, fmt.Errorf("parsing github payload: %w", err)
	}

	if payload.Ref == "" {
		return Event{}, fmt.Errorf("missing ref in github payload")
	}
	if payload.After == "" {
		return Event{}, fmt.Errorf("missing after (sha) in github payload")
	}
	if err := ValidateSHA(payload.After); err != nil {
		return Event{}, fmt.Errorf("github payload: %w", err)
	}

	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")

	return Event{
		Ref:    payload.Ref,
		SHA:    payload.After,
		Branch: branch,
	}, nil
}
