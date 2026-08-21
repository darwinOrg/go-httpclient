package dghttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	nu "net/url"
	"os"
	"strings"
	"time"

	dgctx "github.com/darwinOrg/go-common/context"
	"github.com/darwinOrg/go-common/result"
	dgsys "github.com/darwinOrg/go-common/sys"
	"github.com/darwinOrg/go-common/utils"
	dglogger "github.com/darwinOrg/go-logger"
	"github.com/darwinOrg/go-monitor"
	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/net/http2"
)

const (
	contentTypeHeader               = "Content-Type"
	jsonContentType                 = "application/json; charset=utf-8"
	formUrlEncodedContentType       = "application/x-www-form-urlencoded; charset=utf-8"
	defaultTimeoutSeconds     int64 = 300
	useHttp11                       = "use_http11"
	httpClientKey                   = "httpClient"
)

var (
	HttpTransport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		IdleConnTimeout: time.Duration(int64(time.Second) * defaultTimeoutSeconds),
	}

	Http2Transport = &http2.Transport{
		// So http2.Transport doesn't complain the URL scheme isn't 'https'
		AllowHTTP: true,
		// Pretend we are dialing a TLS endpoint. (Note, we ignore the passed tls.Config)
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			return net.Dial(network, addr)
		},
	}

	Client11         = NewHttpClient(HttpTransport, defaultTimeoutSeconds)
	Client2          = NewHttpClient(Http2Transport, defaultTimeoutSeconds)
	GlobalHttpClient = DefaultHttpClient()
)

type DgHttpClient struct {
	HttpClient              *http.Client
	UseMonitor              bool
	FillHeaderWithDgContext bool
	PrintHeader             bool
	PrintLog                bool
	ResponseCallback        func(ctx *dgctx.DgContext, response *http.Response)
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
		PrintLog:                true,
		FillHeaderWithDgContext: true,
		PrintHeader:             true,
	}
}

func NewRetryableClient() *DgHttpClient {
	retryClient := retryablehttp.NewClient()
	return &DgHttpClient{HttpClient: retryClient.StandardClient(), UseMonitor: dgsys.IsFormalProfile(), PrintHeader: true, PrintLog: true}
}

func (hc *DgHttpClient) DoGet(ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) ([]byte, error) {
	resp, err := hc.DoGetRaw(ctx, url, params, headers)
	if err != nil {
		return nil, err
	}

	return ReadResponse(resp)
}

func (hc *DgHttpClient) DoGetRaw(ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*http.Response, error) {
	if len(params) > 0 {
		if params != nil && len(params) > 0 {
			vs := nu.Values{}
			for k, v := range params {
				vs.Add(k, v)
			}
			url += utils.IfReturn(strings.Contains(url, "?"), "&", "?")
			url += vs.Encode()
		}
	}

	var (
		request *http.Request
		err     error
	)
	request, err = http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, err: %v", url, err)
		return nil, err
	}

	return hc.requestWithHeaders(ctx, request, headers)
}

func (hc *DgHttpClient) DoPostJson(ctx *dgctx.DgContext, url string, params any, headers map[string]string) ([]byte, error) {
	resp, err := hc.DoPostJsonRaw(ctx, url, params, headers)
	if err != nil {
		return nil, err
	}

	return ReadResponse(resp)
}

func (hc *DgHttpClient) DoPostJsonRaw(ctx *dgctx.DgContext, url string, params any, headers map[string]string) (*http.Response, error) {
	var (
		paramsBytes []byte
		err         error
	)
	if params != nil {
		paramsBytes, err = json.Marshal(params)
		if err != nil {
			dglogger.Errorf(ctx, "json marshal error, url: %s, params: %v, err: %v", url, params, err)
			return nil, err
		}
	} else {
		paramsBytes = []byte("{}")
	}

	var request *http.Request
	request, err = http.NewRequest(http.MethodPost, url, bytes.NewBuffer(paramsBytes))
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, params: %v, err: %v", url, params, err)
		return nil, err
	}
	request.Header.Set(contentTypeHeader, jsonContentType)

	return hc.requestWithHeaders(ctx, request, headers)
}

func (hc *DgHttpClient) DoPostFormUrlEncoded(ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) ([]byte, error) {
	var paramsArr []string
	for k, v := range params {
		paramsArr = append(paramsArr, k+"="+v)
	}
	paramsStr := strings.Join(paramsArr, "&")

	var (
		request *http.Request
		err     error
	)
	request, err = http.NewRequest(http.MethodPost, url, strings.NewReader(paramsStr))
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, params: %v, err: %v", url, params, err)
		return nil, err
	}
	request.Header.Set(contentTypeHeader, formUrlEncodedContentType)

	return hc.simpleRequest(ctx, request, headers)
}

func (hc *DgHttpClient) DoPutJsonRaw(ctx *dgctx.DgContext, url string, params any, headers map[string]string) (*http.Response, error) {
	var (
		paramsBytes []byte
		err         error
	)
	if params != nil {
		paramsBytes, err = json.Marshal(params)
		if err != nil {
			dglogger.Errorf(ctx, "json marshal error, url: %s, params: %v, err: %v", url, params, err)
			return nil, err
		}
	} else {
		paramsBytes = []byte("{}")
	}

	var request *http.Request
	request, err = http.NewRequest(http.MethodPut, url, bytes.NewBuffer(paramsBytes))
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, params: %v, err: %v", url, params, err)
		return nil, err
	}
	request.Header.Set(contentTypeHeader, jsonContentType)

	return hc.requestWithHeaders(ctx, request, headers)
}

func (hc *DgHttpClient) DoDeleteRaw(ctx *dgctx.DgContext, url string, headers map[string]string) (*http.Response, error) {
	request, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, err: %v", url, err)
		return nil, err
	}

	return hc.requestWithHeaders(ctx, request, headers)
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
	var (
		request *http.Request
		err     error
	)
	request, err = http.NewRequest(method, url, body)
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, err: %v", url, err)
		return nil, err
	}

	return hc.simpleRequest(ctx, request, headers)
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
		e := "false"
		if err != nil {
			e = "true"
		}
		monitor.HttpClientDuration(urlPath, e, cost.Milliseconds())
	}

	formats := []string{"%s url: %s", "cost: %v"}
	args := []any{request.Method, request.URL.String(), cost}
	if hc.PrintHeader {
		formats = append(formats, "header: %v")
		args = append(args, request.Header)
	}
	bodyString := MustRequestBodyString(request)
	if bodyString != "" {
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

func (hc *DgHttpClient) simpleRequest(ctx *dgctx.DgContext, request *http.Request, headers map[string]string) ([]byte, error) {
	resp, err := hc.requestWithHeaders(ctx, request, headers)
	if err != nil {
		return nil, err
	}

	_, _, body, err := ExtractResponse(ctx, resp)
	return body, err
}

func (hc *DgHttpClient) requestWithHeaders(ctx *dgctx.DgContext, request *http.Request, headers map[string]string) (*http.Response, error) {
	FillHeaders(request, headers)
	return hc.DoRequestRaw(ctx, request)
}

func DoGetToResult[T any](ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*result.Result[T], error) {
	return DoGetToStruct[result.Result[T]](ctx, url, params, headers)
}

func DoGetToResultML[T any](ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*result.ResultML[T], error) {
	return DoGetToStruct[result.ResultML[T]](ctx, url, params, headers)
}

func DoGetToStruct[T any](ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*T, error) {
	resp, err := GetHttpClient(ctx).DoGet(ctx, url, params, headers)
	if err != nil {
		return nil, err
	}

	return utils.ConvertJsonBytesToBean[T](resp)
}

func DoPostJsonToResult[T any](ctx *dgctx.DgContext, url string, params any, headers map[string]string) (*result.Result[T], error) {
	return DoPostJsonToStruct[result.Result[T]](ctx, url, params, headers)
}

func DoPostJsonToResultML[T any](ctx *dgctx.DgContext, url string, params any, headers map[string]string) (*result.ResultML[T], error) {
	return DoPostJsonToStruct[result.ResultML[T]](ctx, url, params, headers)
}

func DoPostJsonToStruct[T any](ctx *dgctx.DgContext, url string, params any, headers map[string]string) (*T, error) {
	resp, err := GetHttpClient(ctx).DoPostJson(ctx, url, params, headers)
	if err != nil {
		return nil, err
	}

	return utils.ConvertJsonBytesToBean[T](resp)
}

func DoPostFormUrlEncodedToStruct[T any](ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*T, error) {
	resp, err := GetHttpClient(ctx).DoPostFormUrlEncoded(ctx, url, params, headers)
	if err != nil {
		return nil, err
	}

	return utils.ConvertJsonBytesToBean[T](resp)
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
