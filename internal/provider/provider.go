package provider

import (
	"fmt"
	"net/http"
	"regexp"
)

var (
	validSHA     = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
	validTagName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,127}$`)
	validRefName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,127}$`)
)

// Event represents a parsed webhook event.
type Event struct {
	Ref       string // e.g. "refs/heads/main"
	SHA       string // commit SHA
	Branch    string // extracted branch name from Ref
	EventType string // e.g. "push", "release", "ping"
}

// Provider handles webhook verification and event parsing for a specific source.
type Provider interface {
	Name() string
	Verify(r *http.Request, body []byte, secret string) error
	ParseEvent(body []byte, eventType string) (Event, error)
}

// ValidateSHA checks that a SHA string is a valid hex commit hash.
func ValidateSHA(sha string) error {
	if !validSHA.MatchString(sha) {
		return fmt.Errorf("invalid SHA format: %q", sha)
	}
	return nil
}

// ValidateTagName checks that a tag name contains only safe characters.
func ValidateTagName(tag string) error {
	if !validTagName.MatchString(tag) {
		return fmt.Errorf("invalid tag name format: %q", tag)
	}
	if containsDotDot(tag) {
		return fmt.Errorf("invalid tag name (contains ..): %q", tag)
	}
	return nil
}

// ValidateRefName checks that a ref name (branch/commitish) contains only safe characters.
func ValidateRefName(ref string) error {
	if !validRefName.MatchString(ref) {
		return fmt.Errorf("invalid ref name format: %q", ref)
	}
	if containsDotDot(ref) {
		return fmt.Errorf("invalid ref name (contains ..): %q", ref)
	}
	return nil
}

func containsDotDot(s string) bool {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '.' && s[i+1] == '.' {
			return true
		}
	}
	return false
}

// Detect identifies the webhook provider from request headers.
// Returns the provider and the event type string.
// Priority: GitHub > Generic.
func Detect(r *http.Request) (Provider, string, error) {
	if eventType := r.Header.Get("X-GitHub-Event"); eventType != "" {
		return &GitHub{}, eventType, nil
	}
	if r.Header.Get("X-Deploq-Token") != "" {
		return &Generic{}, "push", nil
	}
	return nil, "", fmt.Errorf("unable to detect webhook provider: no recognized headers")
}
