package github

import "testing"

func TestParseRemoteURL_SSH(t *testing.T) {
	owner, repo, err := ParseRemoteURL("git@github.com:myorg/myrepo.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != "myorg" {
		t.Errorf("owner = %q, want %q", owner, "myorg")
	}
	if repo != "myrepo" {
		t.Errorf("repo = %q, want %q", repo, "myrepo")
	}
}

func TestParseRemoteURL_SSH_NoGitSuffix(t *testing.T) {
	owner, repo, err := ParseRemoteURL("git@github.com:myorg/myrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != "myorg" || repo != "myrepo" {
		t.Errorf("got %q/%q, want myorg/myrepo", owner, repo)
	}
}

func TestParseRemoteURL_HTTPS(t *testing.T) {
	owner, repo, err := ParseRemoteURL("https://github.com/myorg/myrepo.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != "myorg" {
		t.Errorf("owner = %q, want %q", owner, "myorg")
	}
	if repo != "myrepo" {
		t.Errorf("repo = %q, want %q", repo, "myrepo")
	}
}

func TestParseRemoteURL_HTTPS_NoGitSuffix(t *testing.T) {
	owner, repo, err := ParseRemoteURL("https://github.com/myorg/myrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != "myorg" || repo != "myrepo" {
		t.Errorf("got %q/%q, want myorg/myrepo", owner, repo)
	}
}

func TestParseRemoteURL_Invalid(t *testing.T) {
	tests := []string{
		"not-a-url",
		"ftp://example.com/repo",
		"",
	}
	for _, url := range tests {
		_, _, err := ParseRemoteURL(url)
		if err == nil {
			t.Errorf("expected error for URL %q", url)
		}
	}
}

func TestParseRemoteURL_ValidOwnerRepo(t *testing.T) {
	// Hyphen, dot, underscore should be accepted
	owner, repo, err := ParseRemoteURL("https://github.com/my-org.test/my_repo.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != "my-org.test" || repo != "my_repo" {
		t.Errorf("got %q/%q, want my-org.test/my_repo", owner, repo)
	}
}
