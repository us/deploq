package provider

import (
	"fmt"
	"net/http"
	"regexp"
)

var validSHA = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// Event represents a parsed webhook event.
type Event struct {
	Ref    string // e.g. "refs/heads/main"
	SHA    string // commit SHA
	Branch string // extracted branch name from Ref
}

// Provider handles webhook verification and event parsing for a specific source.
type Provider interface {
	Name() string
	Verify(r *http.Request, body []byte, secret string) error
	ParseEvent(body []byte) (Event, error)
}

// ValidateSHA checks that a SHA string is a valid hex commit hash.
func ValidateSHA(sha string) error {
	if !validSHA.MatchString(sha) {
		return fmt.Errorf("invalid SHA format: %q", sha)
	}
	return nil
}

// Detect identifies the webhook provider from request headers.
// Priority: GitHub > Generic.
func Detect(r *http.Request) (Provider, error) {
	if r.Header.Get("X-GitHub-Event") != "" {
		return &GitHub{}, nil
	}
	if r.Header.Get("X-Deploq-Token") != "" {
		return &Generic{}, nil
	}
	return nil, fmt.Errorf("unable to detect webhook provider: no recognized headers")
}
