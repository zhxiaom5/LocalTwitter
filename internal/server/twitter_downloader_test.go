package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTwitterDownloaderDefaultClientDoesNotCapBodyDownloadTime(t *testing.T) {
	client := TwitterDownloader{}.client()
	if client.Timeout != 0 {
		t.Fatalf("default downloader client timeout = %s, want no whole-body timeout", client.Timeout)
	}
}

func TestTwitterDownloaderRetriesTransientHTTPFailures(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("video"))
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "clip.mp4")
	messages := []string{}
	err := TwitterDownloader{RetrySleeper: func(context.Context, time.Duration) error { return nil }}.downloadWithRetry(context.Background(), server.URL, target, nil, func(message string) {
		messages = append(messages, message)
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "video" || attempts != 3 {
		t.Fatalf("download data=%q attempts=%d", data, attempts)
	}
	if len(messages) == 0 || !strings.Contains(strings.Join(messages, "\n"), "重试") {
		t.Fatalf("retry messages = %#v", messages)
	}
}

func TestTwitterDownloaderDoesNotRetryNonRetryableHTTPFailures(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		t.Run(fmt.Sprintf("status-%d", status), func(t *testing.T) {
			attempts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts++
				http.Error(w, "nope", status)
			}))
			defer server.Close()

			target := filepath.Join(t.TempDir(), "clip.mp4")
			err := TwitterDownloader{RetrySleeper: func(context.Context, time.Duration) error { return nil }}.downloadWithRetry(context.Background(), server.URL, target, nil, nil)
			if err == nil {
				t.Fatal("expected non-retryable status error")
			}
			var retryErr videoDownloadRetryError
			if errors.As(err, &retryErr) {
				t.Fatalf("non-retryable status was wrapped as retry error: %v", err)
			}
			if attempts != 1 {
				t.Fatalf("attempts = %d, want 1", attempts)
			}
		})
	}
}

func TestTwitterLoginUnavailableErrorMentionsProxyWhenConfigured(t *testing.T) {
	err := twitterLoginUnavailableError(UpdateAuthSecret{CookieHeader: "auth_token=auth-one; ct0=ct0-one", Proxy: "socks5://127.0.0.1:1080"})
	if err == nil || !strings.Contains(err.Error(), "代理") {
		t.Fatalf("proxy auth error = %v", err)
	}
	err = twitterLoginUnavailableError(UpdateAuthSecret{CookieHeader: "auth_token=auth-one; ct0=ct0-one"})
	if err == nil || strings.Contains(err.Error(), "代理") {
		t.Fatalf("plain auth error = %v", err)
	}
}

func TestTwitterDownloaderRespectsRetryAfterForRetryableStatus(t *testing.T) {
	attempts := 0
	delays := []time.Duration{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("video"))
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "clip.mp4")
	err := TwitterDownloader{RetrySleeper: func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}}.downloadWithRetry(context.Background(), server.URL, target, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(delays) != 1 || delays[0] != time.Second {
		t.Fatalf("delays = %#v", delays)
	}
}

func TestClassifyUpdateErrorForExhaustedVideoDownloadRetry(t *testing.T) {
	err := videoDownloadRetryError{Attempts: 3, Last: errors.New("context deadline exceeded (Client.Timeout or context cancellation while reading body)")}
	message, detail := classifyUpdateError(err)
	if message != "视频下载超时，已自动重试后仍失败" {
		t.Fatalf("message = %q", message)
	}
	if !strings.Contains(detail, "3") || !strings.Contains(detail, "context deadline exceeded") {
		t.Fatalf("detail = %q", detail)
	}
}
