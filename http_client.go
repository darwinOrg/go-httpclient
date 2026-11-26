package dghttp

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	dgctx "github.com/darwinOrg/go-common/context"
	"github.com/darwinOrg/go-common/result"
	dgsys "github.com/darwinOrg/go-common/sys"
	"github.com/darwinOrg/go-common/utils"
	dglogger "github.com/darwinOrg/go-logger"
	"github.com/darwinOrg/go-monitor"
)

const (
	defaultTimeoutSeconds int64 = 300
	useHttp11                   = "use_http11"
	httpClientKey               = "httpClient"
)

var (
	HttpTransport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		IdleConnTimeout: time.Duration(int64(time.Second) * defaultTimeoutSeconds),
	}

	// Http2Transport 提供 h2c（明文 HTTP/2，prior knowledge）能力。
	// Go 1.24+ 标准库已原生支持 h2c，x/net/http2.Transport 随之废弃
	// （其 DialTLSContext / TLSClientConfig 等字段均标记 Deprecated），
	// 因此改用 http.Transport + Protocols 实现，行为与旧实现一致。
	Http2Transport = newH2cTransport()

	Client11         = NewHttpClient(HttpTransport, defaultTimeoutSeconds)
	Client2          = NewHttpClient(Http2Transport, defaultTimeoutSeconds)
	GlobalHttpClient = DefaultHttpClient()
)

type DgHttpClient struct {
	HttpClient              *http.Client
	UseMonitor              bool
	FillHeaderWithDgContext bool
	PrintLog                bool
	PrintHeader             bool
	PrintBody               bool
	ResponseCallback        func(ctx *dgctx.DgContext, response *http.Response)
	// RetryTimeout 是一次调用（含全部重试）的总预算，0 表示不额外限制。
	// 由 NewRetryableClientWith 根据 RetryConfig.MaxElapsedTime 注入。
	RetryTimeout time.Duration
}

// newH2cTransport 构造一个等价于旧 http2.Transport{AllowHTTP:true,
// DialTLSContext: net.Dial} 的标准库 Transport：
//   - http:// URL 走明文 HTTP/2（h2c prior knowledge）
//   - https:// URL 走标准 TLS + ALPN 协商 h2（旧实现绕过 TLS，属于缺陷，
//     此处修正；依赖该缺陷的内网明文 https 场景应改用 http:// URL）
//
// 关键：Protocols 必须同时关闭 HTTP1 并开启 UnencryptedHTTP2，
// 否则 http:// URL 会回落到 HTTP/1.1，失去 h2c 语义。
func newH2cTransport() *http.Transport {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(false)
	protocols.SetHTTP2(true)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Transport{
		Protocols: protocols,
		// 与 HttpTransport 保持一致：内网服务常用自签证书。
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
}

func DefaultHttpClient() *DgHttpClient {
	return utils.IfReturn(os.Getenv(useHttp11) == "true", Client11, Client2)
}

func NewHttpClient(roundTripper http.RoundTripper, timeoutSeconds int64) *DgHttpClient {
	if timeoutSeconds == 0 {
		timeoutSeconds = defaultTimeoutSeconds
	}

	return &DgHttpClient{
		HttpClient: &http.Client{
			Transport: roundTripper,
			Timeout:   time.Duration(int64(time.Second) * timeoutSeconds),
		},
		UseMonitor:              dgsys.IsFormalProfile(),
		FillHeaderWithDgContext: true,
		PrintLog:                true,
		PrintBody:               true,
	}
}

// NewRetryableClient 创建一个带重试能力的客户端，保留原有签名与默认行为，
// 内部改用带抖动退避、Retry-After 与总预算的策略。
func NewRetryableClient() *DgHttpClient {
	return NewRetryableClientWithOptions()
}

func (hc *DgHttpClient) DoGet(ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) ([]byte, error) {
	resp, err := hc.DoGetRaw(ctx, url, params, headers)
	if err != nil {
		return nil, err
	}

	return ReadResponse(resp)
}

func (hc *DgHttpClient) DoGetRaw(ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*http.Response, error) {
	url = AppendUrlParams(url, params)
	request, err := newRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, err: %v", url, err)
		return nil, err
	}

	FillHeaders(request, headers)
	return hc.DoRequestRaw(ctx, request)
}

func (hc *DgHttpClient) DoPostJson(ctx *dgctx.DgContext, url string, params any, headers map[string]string) ([]byte, error) {
	resp, err := hc.DoPostJsonRaw(ctx, url, params, headers)
	if err != nil {
		return nil, err
	}

	return ReadResponse(resp)
}

func (hc *DgHttpClient) DoPostJsonRaw(ctx *dgctx.DgContext, url string, params any, headers map[string]string) (*http.Response, error) {
	request, err := BuildJsonRequest(ctx, http.MethodPost, url, params, headers)
	if err != nil {
		return nil, err
	}

	return hc.DoRequestRaw(ctx, request)
}

func (hc *DgHttpClient) DoPutJsonRaw(ctx *dgctx.DgContext, url string, params any, headers map[string]string) (*http.Response, error) {
	request, err := BuildJsonRequest(ctx, http.MethodPut, url, params, headers)
	if err != nil {
		return nil, err
	}

	return hc.DoRequestRaw(ctx, request)
}

func (hc *DgHttpClient) DoPostFormUrlEncoded(ctx *dgctx.DgContext, url string, params, headers map[string]string) ([]byte, error) {
	var paramsArr []string
	for k, v := range params {
		paramsArr = append(paramsArr, k+"="+v)
	}
	paramsStr := strings.Join(paramsArr, "&")

	request, err := newRequest(ctx, http.MethodPost, url, strings.NewReader(paramsStr))
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, params: %v, err: %v", url, params, err)
		return nil, err
	}
	FillHeaders(request, headers)
	request.Header.Set(contentTypeHeader, formUrlEncodedContentType)

	_, _, data, err := hc.DoRequest(ctx, request)
	return data, err
}

func (hc *DgHttpClient) DoDeleteRaw(ctx *dgctx.DgContext, url string, headers map[string]string) (*http.Response, error) {
	request, err := newRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, err: %v", url, err)
		return nil, err
	}

	FillHeaders(request, headers)
	return hc.DoRequestRaw(ctx, request)
}

func (hc *DgHttpClient) DoUploadBodyFromLocalFile(ctx *dgctx.DgContext, method, url, filePath string, headers map[string]string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		dglogger.Errorf(ctx, "error opening file: %s", filePath)
		return nil, errors.New("error opening file")
	}
	defer func() {
		_ = file.Close()
	}()

	// 创建请求 body buffer
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// 添加文件字段
	part, err := writer.CreateFormFile("file", filePath)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return nil, err
	}

	// 关闭 multipart writer（必须调用，否则 boundary 不完整）
	err = writer.Close()
	if err != nil {
		return nil, err
	}

	if headers != nil {
		headers[contentTypeHeader] = writer.FormDataContentType()
	} else {
		headers = make(map[string]string)
		headers[contentTypeHeader] = writer.FormDataContentType()
	}

	return hc.DoUploadBody(ctx, method, url, &body, headers)
}

func (hc *DgHttpClient) DoUploadBody(ctx *dgctx.DgContext, method string, url string, body io.Reader, headers map[string]string) ([]byte, error) {
	request, err := newRequest(ctx, method, url, body)
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, err: %v", url, err)
		return nil, err
	}

	FillHeaders(request, headers)
	_, _, data, err := hc.DoRequest(ctx, request)
	return data, err
}

func (hc *DgHttpClient) DoRequest(ctx *dgctx.DgContext, request *http.Request) (int, map[string][]string, []byte, error) {
	response, err := hc.DoRequestRaw(ctx, request)
	if err != nil {
		return http.StatusInternalServerError, nil, nil, err
	}

	return ExtractResponse(ctx, response)
}

func (hc *DgHttpClient) DoRequestRaw(ctx *dgctx.DgContext, request *http.Request) (*http.Response, error) {
	start := time.Now()
	urlPath := request.URL.Path

	var bodyString string
	if hc.PrintBody {
		bodyString = MustRequestBodyString(request)
	}
	if hc.UseMonitor {
		monitor.HttpClientCounter(urlPath)
	}
	if hc.FillHeaderWithDgContext {
		FillHeadersWithDgContext(ctx, request.Header)
	}

	request.Header.Set("User-Agent", "")
	response, err := hc.HttpClient.Do(request)
	cost := time.Since(start)
	if hc.UseMonitor {
		monitor.HttpClientDuration(urlPath, utils.IfReturn(err == nil, "false", "true"), cost.Milliseconds())
	}

	formats := []string{"%s %s", "cost: %s"}
	args := []any{request.Method, request.URL.String(), utils.SimpleFormatDuration(cost)}
	if hc.PrintHeader && len(request.Header) > 0 {
		formats = append(formats, "header: %v")
		args = append(args, request.Header)
	}
	if hc.PrintBody && bodyString != "" {
		formats = append(formats, "body: %s")
		args = append(args, bodyString)
	}
	if err != nil {
		formats = append(formats, "err: %v")
		args = append(args, err)
		dglogger.Errorf(ctx, strings.Join(formats, " | "), args...)
		return response, err
	}
	if hc.PrintLog && !ctx.NotPrintLog {
		dglogger.Infof(ctx, strings.Join(formats, " | "), args...)
	}

	if hc.ResponseCallback != nil {
		hc.ResponseCallback(ctx, response)
	}

	return response, err
}

// ---------------------------------------------------------------- 自有泛型方法（Go 1.27+ 方法类型参数）
// 这七个方法挂在 *DgHttpClient 上，使用调用方持有的客户端实例发起请求并解析响应。
// 方法接收者决定连接池 / 超时 / 重试策略；ctx 仍用于日志、header 注入与链路追踪。

func (hc *DgHttpClient) DoGetToStruct[T any](ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*T, error) {
	resp, err := hc.DoGet(ctx, url, params, headers)
	if err != nil {
		return nil, err
	}

	return utils.ConvertJsonBytesToBean[T](resp)
}

func (hc *DgHttpClient) DoGetToResult[T any](ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*result.Result[T], error) {
	return hc.DoGetToStruct[result.Result[T]](ctx, url, params, headers)
}

func (hc *DgHttpClient) DoGetToResultML[T any](ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*result.ResultML[T], error) {
	return hc.DoGetToStruct[result.ResultML[T]](ctx, url, params, headers)
}

func (hc *DgHttpClient) DoPostJsonToStruct[T any](ctx *dgctx.DgContext, url string, params any, headers map[string]string) (*T, error) {
	resp, err := hc.DoPostJson(ctx, url, params, headers)
	if err != nil {
		return nil, err
	}

	return utils.ConvertJsonBytesToBean[T](resp)
}

func (hc *DgHttpClient) DoPostJsonToResult[T any](ctx *dgctx.DgContext, url string, params any, headers map[string]string) (*result.Result[T], error) {
	return hc.DoPostJsonToStruct[result.Result[T]](ctx, url, params, headers)
}

func (hc *DgHttpClient) DoPostJsonToResultML[T any](ctx *dgctx.DgContext, url string, params any, headers map[string]string) (*result.ResultML[T], error) {
	return hc.DoPostJsonToStruct[result.ResultML[T]](ctx, url, params, headers)
}

func (hc *DgHttpClient) DoPostFormUrlEncodedToStruct[T any](ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*T, error) {
	resp, err := hc.DoPostFormUrlEncoded(ctx, url, params, headers)
	if err != nil {
		return nil, err
	}

	return utils.ConvertJsonBytesToBean[T](resp)
}

// ---------------------------------------------------------------- 包级兼容包装
// 保留原有签名，委托给 ctx 上绑定的客户端，兼容既有跨仓库调用。

func DoGetToResult[T any](ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*result.Result[T], error) {
	return GetHttpClient(ctx).DoGetToResult[T](ctx, url, params, headers)
}

func DoGetToResultML[T any](ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*result.ResultML[T], error) {
	return GetHttpClient(ctx).DoGetToResultML[T](ctx, url, params, headers)
}

func DoGetToStruct[T any](ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*T, error) {
	return GetHttpClient(ctx).DoGetToStruct[T](ctx, url, params, headers)
}

func DoPostJsonToResult[T any](ctx *dgctx.DgContext, url string, params any, headers map[string]string) (*result.Result[T], error) {
	return GetHttpClient(ctx).DoPostJsonToResult[T](ctx, url, params, headers)
}

func DoPostJsonToResultML[T any](ctx *dgctx.DgContext, url string, params any, headers map[string]string) (*result.ResultML[T], error) {
	return GetHttpClient(ctx).DoPostJsonToResultML[T](ctx, url, params, headers)
}

func DoPostJsonToStruct[T any](ctx *dgctx.DgContext, url string, params any, headers map[string]string) (*T, error) {
	return GetHttpClient(ctx).DoPostJsonToStruct[T](ctx, url, params, headers)
}

func DoPostFormUrlEncodedToStruct[T any](ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*T, error) {
	return GetHttpClient(ctx).DoPostFormUrlEncodedToStruct[T](ctx, url, params, headers)
}

func SetHttpClient(ctx *dgctx.DgContext, httpClient *DgHttpClient) {
	ctx.SetExtraKeyValue(httpClientKey, httpClient)
}

func GetHttpClient(ctx *dgctx.DgContext) *DgHttpClient {
	httpClient := ctx.GetExtraValue(httpClientKey)
	if httpClient == nil {
		return GlobalHttpClient
	}

	return httpClient.(*DgHttpClient)
}
