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

// ParseEvent extracts event data based on eventType.
func (g *GitHub) ParseEvent(body []byte, eventType string) (Event, error) {
	switch eventType {
	case "push":
		return g.parsePushEvent(body)
	case "release":
		return g.parseReleaseEvent(body)
	case "ping":
		return Event{EventType: "ping"}, nil
	default:
		return Event{}, fmt.Errorf("unsupported event type: %q", eventType)
	}
}

func (g *GitHub) parsePushEvent(body []byte) (Event, error) {
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
		Ref:       payload.Ref,
		SHA:       payload.After,
		Branch:    branch,
		EventType: "push",
	}, nil
}

func (g *GitHub) parseReleaseEvent(body []byte) (Event, error) {
	var payload struct {
		Action  string `json:"action"`
		Release struct {
			TagName         string `json:"tag_name"`
			TargetCommitish string `json:"target_commitish"`
		} `json:"release"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Event{}, fmt.Errorf("parsing github release payload: %w", err)
	}

	if payload.Action != "published" {
		return Event{}, fmt.Errorf("ignoring release action %q (only published supported)", payload.Action)
	}
	if payload.Release.TagName == "" {
		return Event{}, fmt.Errorf("missing tag_name in release payload")
	}
	if err := ValidateTagName(payload.Release.TagName); err != nil {
		return Event{}, fmt.Errorf("release payload: %w", err)
	}
	if payload.Release.TargetCommitish != "" {
		if err := ValidateRefName(payload.Release.TargetCommitish); err != nil {
			return Event{}, fmt.Errorf("release payload: %w", err)
		}
	}

	return Event{
		Ref:       "refs/tags/" + payload.Release.TagName,
		Branch:    payload.Release.TargetCommitish,
		EventType: "release",
	}, nil
}
