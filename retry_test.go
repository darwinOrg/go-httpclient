package dghttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	dgctx "github.com/darwinOrg/go-common/context"
)

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"garbage", 0},
		{"2", 2 * time.Second},
	}
	for _, tc := range tests {
		if got := parseRetryAfter(tc.in); got != tc.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}

	future := time.Now().Add(3 * time.Second).UTC().Format(http.TimeFormat)
	got := parseRetryAfter(future)
	if got <= 2*time.Second || got > 4*time.Second {
		t.Errorf("HTTP-date Retry-After = %v, want ~3s", got)
	}
}

func TestBackoffFullJitterBounds(t *testing.T) {
	cfg := buildRetryConfig([]RetryOption{WithWaitRange(100*time.Millisecond, 2*time.Second)})
	backoff := makeBackoff(cfg)

	for attempt := 0; attempt < 6; attempt++ {
		for i := 0; i < 200; i++ {
			got := backoff(100*time.Millisecond, 2*time.Second, attempt, nil)
			if got < 0 || got > 2*time.Second {
				t.Fatalf("attempt=%d backoff=%v out of [0,2s]", attempt, got)
			}
		}
	}
}

func TestBackoffRespectsRetryAfter(t *testing.T) {
	cfg := buildRetryConfig(nil)
	backoff := makeBackoff(cfg)
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{"Retry-After": []string{"1"}},
	}
	got := backoff(cfg.WaitMin, cfg.WaitMax, 3, resp)
	if got != time.Second {
		t.Errorf("Retry-After backoff = %v, want 1s", got)
	}

	disabled := buildRetryConfig([]RetryOption{WithRespectRetryAfter(false)})
	if got := makeBackoff(disabled)(cfg.WaitMin, cfg.WaitMax, 3, resp); got == time.Second {
		t.Error("WithRespectRetryAfter(false) still honored Retry-After")
	}
}

func TestCheckRetryStatusCodes(t *testing.T) {
	check := makeCheckRetry(buildRetryConfig(nil))
	cases := map[int]bool{
		http.StatusOK:                  false,
		http.StatusBadRequest:          false,
		http.StatusNotFound:            false,
		http.StatusNotImplemented:      false,
		http.StatusRequestTimeout:      true,
		http.StatusTooManyRequests:     true,
		http.StatusInternalServerError: true,
		http.StatusBadGateway:          true,
		http.StatusServiceUnavailable:  true,
		http.StatusGatewayTimeout:      true,
	}
	for code, want := range cases {
		got, err := check(context.Background(), &http.Response{StatusCode: code}, nil)
		if err != nil {
			t.Fatalf("code=%d unexpected err: %v", code, err)
		}
		if got != want {
			t.Errorf("code=%d retry=%v, want %v", code, got, want)
		}
	}

	got, _ := check(context.Background(), &http.Response{StatusCode: 500}, errors.New("connection reset"))
	if !got {
		t.Error("transport errors must be retryable")
	}
}

func TestCheckRetryStopsOnCanceledContext(t *testing.T) {
	check := makeCheckRetry(buildRetryConfig(nil))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := check(ctx, &http.Response{StatusCode: 500}, nil)
	if got || !errors.Is(err, context.Canceled) {
		t.Errorf("canceled ctx: retry=%v err=%v, want retry=false + context.Canceled", got, err)
	}
}

func TestPassthroughOnExhaustedKeepsStatus(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusServiceUnavailable}
	got, err := passthroughOnExhausted(resp, errors.New("unexpected HTTP status"), 4)
	if got != resp || err != nil {
		t.Errorf("exhausted 503 should pass response with nil err, got resp=%p err=%v", got, err)
	}
}

func TestPassthroughOnExhaustedPropagatesDeadline(t *testing.T) {
	got, err := passthroughOnExhausted(nil, context.DeadlineExceeded, 4)
	if got != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("deadline must propagate, got resp=%p err=%v", got, err)
	}
}

func TestRetryClientRetriesThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"success":false}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":"ok"}`))
	}))
	defer srv.Close()

	client := NewRetryableClientWithOptions(
		WithMaxAttempts(4),
		WithWaitRange(time.Millisecond, 5*time.Millisecond),
		WithMaxElapsedTime(2*time.Second),
	)
	client.PrintLog = false

	ctx := dgctx.SimpleDgContext()
	resp, err := client.DoGetRaw(ctx, srv.URL, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("calls = %d, want 3", n)
	}
}

func TestRetryClientNoRetryOnBusinessFailure(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":false,"message":"biz error"}`))
	}))
	defer srv.Close()

	client := NewRetryableClientWithOptions(
		WithMaxAttempts(4),
		WithWaitRange(time.Millisecond, 5*time.Millisecond),
	)
	client.PrintLog = false

	resp, err := client.DoGetRaw(dgctx.SimpleDgContext(), srv.URL, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if n := calls.Load(); n != 1 {
		t.Errorf("business failure retried %d times, want 1", n)
	}
}

func TestRetryClientHonorsTotalBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"success":false}`))
	}))
	defer srv.Close()

	client := NewRetryableClientWithOptions(
		WithMaxAttempts(100),
		WithWaitRange(10*time.Millisecond, 20*time.Millisecond),
		WithMaxElapsedTime(150*time.Millisecond),
	)
	client.PrintLog = false

	start := time.Now()
	_, err := client.DoGetRaw(dgctx.SimpleDgContext(), srv.URL, nil, nil)
	elapsed := time.Since(start)

	if elapsed > 800*time.Millisecond {
		t.Errorf("elapsed %v exceeds total budget with margin", elapsed)
	}
	if err == nil {
		t.Error("expected deadline error after budget exhausted")
	}
}

func TestContextPropagation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	inner, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	ctx := dgctx.SimpleDgContext()
	ctx.SetInnerContext(inner)

	client := NewHttpClient(HttpTransport.Clone(), 5)
	client.PrintLog = false

	start := time.Now()
	_, err := client.DoGetRaw(ctx, srv.URL, nil, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected context deadline error")
	}
	if elapsed > 600*time.Millisecond {
		t.Errorf("context cancellation not propagated, elapsed=%v", elapsed)
	}
}

func TestAllDefaultsInRange(t *testing.T) {
	cfg := buildRetryConfig(nil)
	if cfg.MaxAttempts != defaultRetryAttempts {
		t.Errorf("MaxAttempts = %d", cfg.MaxAttempts)
	}
	if cfg.MaxElapsedTime != defaultRetryMaxElapsed {
		t.Errorf("MaxElapsedTime = %v", cfg.MaxElapsedTime)
	}
	if err := fmt.Sprintf("%d", cfg.MaxAttempts); err != "4" {
		t.Errorf("unexpected attempts: %s", err)
	}
}
