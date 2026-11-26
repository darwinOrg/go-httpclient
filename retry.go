package dghttp

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net/http"
	"time"

	dgsys "github.com/darwinOrg/go-common/sys"
	"github.com/hashicorp/go-retryablehttp"
)

const (
	defaultRetryAttempts   = 4
	defaultRetryWaitMin    = 500 * time.Millisecond
	defaultRetryWaitMax    = 8 * time.Second
	defaultRetryMaxElapsed = 55 * time.Second
)

// RetryConfig 描述一次调用的重试策略。
// 所有字段都有可用默认值，通常只需通过 RetryOption 覆盖关心的部分。
type RetryConfig struct {
	// MaxAttempts 是请求总次数上限（含首次），必须 >= 1。
	MaxAttempts int
	// WaitMin / WaitMax 是 full-jitter 指数退避的上下界。
	WaitMin time.Duration
	WaitMax time.Duration
	// MaxElapsedTime 是整个调用（含全部重试与退避）的预算。
	// 超过后上下文被取消，retryablehttp 会立即停止重试。0 表示不限制。
	MaxElapsedTime time.Duration
	// RespectRetryAfter 控制是否遵守 429/503 响应的 Retry-After 头。
	RespectRetryAfter bool
	// RetryableStatuses 是除内置 408/429/5xx 之外需要重试的状态码。
	RetryableStatuses []int
}

// DefaultRetryConfig 返回面向实时接口的保守默认值：
// 4 次尝试、0.5~8s full-jitter 退避、55s 总预算、遵守 Retry-After。
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:       defaultRetryAttempts,
		WaitMin:           defaultRetryWaitMin,
		WaitMax:           defaultRetryWaitMax,
		MaxElapsedTime:    defaultRetryMaxElapsed,
		RespectRetryAfter: true,
	}
}

// RetryOption 用于覆盖 RetryConfig 的单个字段。
type RetryOption func(*RetryConfig)

func WithMaxAttempts(n int) RetryOption {
	return func(c *RetryConfig) {
		if n >= 1 {
			c.MaxAttempts = n
		}
	}
}

func WithWaitRange(min, max time.Duration) RetryOption {
	return func(c *RetryConfig) {
		if min > 0 {
			c.WaitMin = min
		}
		if max >= min && max > 0 {
			c.WaitMax = max
		}
	}
}

// WithMaxElapsedTime 设置整个调用的总预算。传 0 表示禁用总预算。
func WithMaxElapsedTime(d time.Duration) RetryOption {
	return func(c *RetryConfig) {
		if d >= 0 {
			c.MaxElapsedTime = d
		}
	}
}

// WithRespectRetryAfter 控制是否遵守服务端的 Retry-After 建议。
func WithRespectRetryAfter(enabled bool) RetryOption {
	return func(c *RetryConfig) {
		c.RespectRetryAfter = enabled
	}
}

// WithRetryableStatus 追加需要重试的状态码（如 409）。
func WithRetryableStatus(codes ...int) RetryOption {
	return func(c *RetryConfig) {
		c.RetryableStatuses = append(c.RetryableStatuses, codes...)
	}
}

// buildRetryConfig 应用选项并做边界收敛，避免非法配置放大重试压力。
func buildRetryConfig(opts []RetryOption) RetryConfig {
	cfg := DefaultRetryConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.MaxAttempts < 1 {
		cfg.MaxAttempts = 1
	}
	if cfg.WaitMin <= 0 {
		cfg.WaitMin = defaultRetryWaitMin
	}
	if cfg.WaitMax < cfg.WaitMin {
		cfg.WaitMax = cfg.WaitMin
	}
	return cfg
}

// makeCheckRetry 生成重试判定策略。
// 重试传输错误、408/429 与 5xx（501 除外）；业务失败（200 + success=false）
// 是 HTTP 200，不会进入重试，与既有 API 契约保持一致。
func makeCheckRetry(cfg RetryConfig) retryablehttp.CheckRetry {
	extra := make(map[int]struct{}, len(cfg.RetryableStatuses))
	for _, code := range cfg.RetryableStatuses {
		extra[code] = struct{}{}
	}

	return func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		if err != nil {
			return true, nil
		}
		if resp == nil {
			return false, nil
		}
		if _, ok := extra[resp.StatusCode]; ok {
			return true, nil
		}
		switch resp.StatusCode {
		case http.StatusRequestTimeout, http.StatusTooManyRequests,
			http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true, nil
		}
		return false, nil
	}
}

// makeBackoff 生成 full-jitter 指数退避。
// 相比 retryablehttp 默认的无抖动指数退避，full jitter 能把同时失败的
// 多个请求打散到 [0, ceiling) 区间，避免同步重试波峰。
func makeBackoff(cfg RetryConfig) retryablehttp.Backoff {
	return func(min, max time.Duration, attempt int, resp *http.Response) time.Duration {
		if cfg.RespectRetryAfter && resp != nil &&
			(resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable) {
			if sleep := parseRetryAfter(resp.Header.Get("Retry-After")); sleep > 0 {
				return clampDuration(sleep, min, max)
			}
		}

		if min <= 0 {
			min = defaultRetryWaitMin
		}
		if max < min {
			max = min
		}
		ceiling := time.Duration(float64(min) * math.Pow(2, float64(attempt)))
		if ceiling > max || ceiling <= 0 {
			ceiling = max
		}
		return time.Duration(rand.Int63n(int64(ceiling) + 1))
	}
}

// parseRetryAfter 支持 RFC 7231 的两种格式：延迟秒数与 HTTP 日期。
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := time.ParseDuration(value + "s"); err == nil {
		return seconds
	}
	if date, err := http.ParseTime(value); err == nil {
		return time.Until(date)
	}
	return 0
}

func clampDuration(d, min, max time.Duration) time.Duration {
	if d < min {
		return min
	}
	if d > max {
		return max
	}
	return d
}

// passthroughOnExhausted 在重试耗尽时保留最后一次响应，让调用方仍能读取
// 状态码与响应体。这与 ExtractResponse 的既有语义一致：HTTP >= 300 时
// 不把状态码包装成 error，由调用方决定如何解释。
func passthroughOnExhausted(resp *http.Response, err error, _ int) (*http.Response, error) {
	// 上下文取消/超时必须原样返回，否则调用方无法区分"服务端 503"与"本地预算耗尽"。
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return resp, err
	}
	if resp != nil {
		return resp, nil
	}
	return nil, err
}

// silentLogger 关闭 retryablehttp 的 stderr 日志，统一由 DgHttpClient 记录。
type silentLogger struct{}

func (silentLogger) Printf(string, ...interface{}) {}

// NewRetryableClientWithOptions 创建一个带重试能力的客户端，使用库默认的
// HttpTransport 副本。不传选项时等价于 NewRetryableClient()。
func NewRetryableClientWithOptions(opts ...RetryOption) *DgHttpClient {
	return NewRetryableClientWith(HttpTransport.Clone(), 0, opts...)
}

// NewRetryableClientWith 创建一个带重试能力的客户端。
// roundTripper 为 nil 时使用 retryablehttp 自带的池化 Transport；
// timeoutSeconds 为 0 时不设置 http.Client.Timeout，仅由 RetryConfig 的
// MaxElapsedTime 通过 context 控制总预算。
func NewRetryableClientWith(roundTripper http.RoundTripper, timeoutSeconds int64, opts ...RetryOption) *DgHttpClient {
	cfg := buildRetryConfig(opts)

	rc := retryablehttp.NewClient()
	rc.Logger = silentLogger{}
	rc.RetryMax = cfg.MaxAttempts - 1
	rc.RetryWaitMin = cfg.WaitMin
	rc.RetryWaitMax = cfg.WaitMax
	rc.CheckRetry = makeCheckRetry(cfg)
	rc.Backoff = makeBackoff(cfg)
	rc.ErrorHandler = passthroughOnExhausted
	if roundTripper != nil {
		rc.HTTPClient = &http.Client{Transport: roundTripper}
	}

	standard := rc.StandardClient()
	// http.Client.Timeout 覆盖整个 Do() 周期（含所有重试与退避），
	// 且会作为 request deadline 传入 CheckRetry，是总预算最可靠的落点。
	// 显式 timeoutSeconds 优先；否则用 RetryConfig.MaxElapsedTime。
	if timeoutSeconds > 0 {
		standard.Timeout = time.Duration(timeoutSeconds) * time.Second
	} else if cfg.MaxElapsedTime > 0 {
		standard.Timeout = cfg.MaxElapsedTime
	}

	return &DgHttpClient{
		HttpClient:              standard,
		UseMonitor:              dgsys.IsFormalProfile(),
		FillHeaderWithDgContext: true,
		PrintLog:                true,
		PrintBody:               true,
		RetryTimeout:            cfg.MaxElapsedTime,
	}
}

// NewHttpClientWithRetry 创建一个使用自定义 Transport 的重试客户端，
// 便于复用针对特定服务调优的连接池（如更短的 IdleConnTimeout）。
func NewHttpClientWithRetry(roundTripper http.RoundTripper, timeoutSeconds int64, opts ...RetryOption) *DgHttpClient {
	return NewRetryableClientWith(roundTripper, timeoutSeconds, opts...)
}
