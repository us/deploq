package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testChecker(token string, url string) *StatusChecker {
	sc := NewStatusChecker(token)
	sc.baseURL = url
	sc.backoff = make([]time.Duration, 100) // no delay in tests, enough slots to avoid 30s fallback
	return sc
}

func TestWaitForSuccess_Immediate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"state":"success"}`)
	}))
	defer srv.Close()

	sc := testChecker("test-token", srv.URL)

	err := sc.WaitForSuccess(context.Background(), "owner", "repo", "abc123", 30*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitForSuccess_PendingThenSuccess(t *testing.T) {
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		n := calls.Add(1)
		if n <= 2 {
			fmt.Fprint(w, `{"state":"pending"}`)
		} else {
			fmt.Fprint(w, `{"state":"success"}`)
		}
	}))
	defer srv.Close()

	sc := testChecker("test-token", srv.URL)

	err := sc.WaitForSuccess(context.Background(), "owner", "repo", "abc123", 2*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 API calls, got %d", calls.Load())
	}
}

func TestWaitForSuccess_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"state":"failure"}`)
	}))
	defer srv.Close()

	sc := testChecker("test-token", srv.URL)

	err := sc.WaitForSuccess(context.Background(), "owner", "repo", "abc123", 30*time.Second)
	if err == nil {
		t.Fatal("expected error for failure state")
	}
}

func TestWaitForSuccess_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"state":"pending"}`)
	}))
	defer srv.Close()

	sc := testChecker("test-token", srv.URL)

	err := sc.WaitForSuccess(context.Background(), "owner", "repo", "abc123", 1*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestWaitForSuccess_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"state":"pending"}`)
	}))
	defer srv.Close()

	sc := testChecker("test-token", srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := sc.WaitForSuccess(ctx, "owner", "repo", "abc123", 1*time.Minute)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestWaitForSuccess_AuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer my-gh-token" {
			t.Errorf("Authorization = %q, want %q", auth, "Bearer my-gh-token")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"state":"success"}`)
	}))
	defer srv.Close()

	sc := testChecker("my-gh-token", srv.URL)

	err := sc.WaitForSuccess(context.Background(), "owner", "repo", "abc123", 30*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitForSuccess_TransientErrorThenSuccess(t *testing.T) {
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"message":"internal error"}`)
		} else {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"state":"success"}`)
		}
	}))
	defer srv.Close()

	sc := testChecker("test-token", srv.URL)

	err := sc.WaitForSuccess(context.Background(), "owner", "repo", "abc123", 30*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 API calls (2 transient + 1 success), got %d", calls.Load())
	}
}

func TestWaitForSuccess_TooManyTransientErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, `{"message":"bad gateway"}`)
	}))
	defer srv.Close()

	sc := testChecker("test-token", srv.URL)

	err := sc.WaitForSuccess(context.Background(), "owner", "repo", "abc123", 30*time.Second)
	if err == nil {
		t.Fatal("expected error after too many transient errors")
	}
}
