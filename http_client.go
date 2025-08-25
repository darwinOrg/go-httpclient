package dghttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
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
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/net/http2"
)

const (
	originalUrl                     = "originalUrl"
	jsonContentType                 = "application/json; charset=utf-8"
	formUrlEncodedContentType       = "application/x-www-form-urlencoded; charset=utf-8"
	defaultTimeoutSeconds     int64 = 300
	useHttp11                       = "use_http11"
	httpClientKey                   = "httpClient"
)

var (
	HttpTransport = otelhttp.NewTransport(&http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		IdleConnTimeout: time.Duration(int64(time.Second) * defaultTimeoutSeconds),
	}, OtelHttpSpanNameFormatterOption)

	Http2Transport = otelhttp.NewTransport(&http2.Transport{
		// So http2.Transport doesn't complain the URL scheme isn't 'https'
		AllowHTTP: true,
		// Pretend we are dialing a TLS endpoint. (Note, we ignore the passed tls.Config)
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			return net.Dial(network, addr)
		},
	}, OtelHttpSpanNameFormatterOption)

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

func (hc *DgHttpClient) DoGet(ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) ([]byte, error) {
	resp, err := hc.DoGetRaw(ctx, url, params, headers)
	if err != nil {
		return nil, err
	}

	return ReadResponse(resp)
}

func (hc *DgHttpClient) DoGetRaw(ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*http.Response, error) {
	ctx.SetExtraKeyValue(originalUrl, url)
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

	if ctx.GetInnerContext() != nil {
		request, err = http.NewRequestWithContext(ContextWithBaggage(ctx, ctx.GetInnerContext()), http.MethodGet, url, nil)
	} else {
		request, err = http.NewRequestWithContext(ContextWithBaggage(ctx, context.Background()), http.MethodGet, url, nil)
	}
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
	ctx.SetExtraKeyValue(originalUrl, url)
	paramsBytes, err := dglogger.Json(params)
	if err != nil {
		dglogger.Errorf(ctx, "json marshal error, url: %s, params: %v, err: %v", url, params, err)
		return nil, err
	}
	if hc.PrintLog && !ctx.NotPrintLog {
		dglogger.Infof(ctx, "post request, url: %s, params: %v", url, string(paramsBytes))
	}

	var request *http.Request
	if ctx.GetInnerContext() != nil {
		request, err = http.NewRequestWithContext(ContextWithBaggage(ctx, ctx.GetInnerContext()), http.MethodPost, url, bytes.NewBuffer(paramsBytes))
	} else {
		request, err = http.NewRequestWithContext(ContextWithBaggage(ctx, context.Background()), http.MethodPost, url, bytes.NewBuffer(paramsBytes))
	}
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, params: %v, err: %v", url, params, err)
		return nil, err
	}
	request.Header.Set("Content-Type", jsonContentType)

	return hc.requestWithHeaders(ctx, request, headers)
}

func (hc *DgHttpClient) DoPostFormUrlEncoded(ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) ([]byte, error) {
	ctx.SetExtraKeyValue(originalUrl, url)
	var paramsArr []string
	for k, v := range params {
		paramsArr = append(paramsArr, k+"="+v)
	}
	paramsStr := strings.Join(paramsArr, "&")
	if hc.PrintLog && !ctx.NotPrintLog {
		dglogger.Infof(ctx, "post request, url: %s, params: %s", url, paramsStr)
	}

	var (
		request *http.Request
		err     error
	)
	if ctx.GetInnerContext() != nil {
		request, err = http.NewRequestWithContext(ContextWithBaggage(ctx, ctx.GetInnerContext()), http.MethodPost, url, strings.NewReader(paramsStr))
	} else {
		request, err = http.NewRequestWithContext(ContextWithBaggage(ctx, context.Background()), http.MethodPost, url, strings.NewReader(paramsStr))
	}
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, params: %v, err: %v", url, params, err)
		return nil, err
	}
	request.Header.Set("Content-Type", formUrlEncodedContentType)

	return hc.simpleRequest(ctx, request, headers)
}

func (hc *DgHttpClient) DoUploadBodyFromLocalFile(ctx *dgctx.DgContext, method string, url string, filePath string) ([]byte, error) {
	fh, err := os.Open(filePath)
	if err != nil {
		dglogger.Errorf(ctx, "error opening file: %s", filePath)
		return nil, errors.New("error opening file")
	}
	defer func() {
		_ = fh.Close()
	}()

	return hc.DoUploadBody(ctx, method, url, fh)
}

func (hc *DgHttpClient) DoUploadBody(ctx *dgctx.DgContext, method string, url string, body io.Reader) ([]byte, error) {
	ctx.SetExtraKeyValue(originalUrl, url)
	dglogger.Infof(ctx, "upload, url: %s", url)

	var (
		request *http.Request
		err     error
	)
	if ctx.GetInnerContext() != nil {
		request, err = http.NewRequestWithContext(ContextWithBaggage(ctx, ctx.GetInnerContext()), method, url, body)
	} else {
		request, err = http.NewRequestWithContext(ContextWithBaggage(ctx, context.Background()), method, url, body)
	}
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, err: %v", url, err)
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return hc.simpleRequest(ctx, request, nil)
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
	if hc.UseMonitor {
		if ctx.GetExtraValue(originalUrl) != nil {
			monitor.HttpClientCounter(ctx.GetExtraValue(originalUrl).(string))
		} else {
			monitor.HttpClientCounter(request.URL.String())
		}
	}

	if hc.FillHeaderWithDgContext {
		FillHeadersWithDgContext(ctx, request.Header)
	}
	if hc.PrintHeader {
		dglogger.Infof(ctx, "httpclient request headers: %v", request.Header)
	}

	response, err := hc.HttpClient.Do(request)

	cost := time.Since(start)
	if hc.UseMonitor {
		e := "false"
		if err != nil {
			e = "true"
		}
		if ctx.GetExtraValue(originalUrl) != nil {
			monitor.HttpClientDuration(ctx.GetExtraValue(originalUrl).(string), e, cost.Milliseconds())
		} else {
			monitor.HttpClientDuration(request.URL.String(), e, cost.Milliseconds())
		}
	}
	if err != nil {
		dglogger.Errorf(ctx, "call url: %s, cost: %v err: %v", request.URL.String(), cost, err)
		return response, err
	} else if hc.PrintLog && !ctx.NotPrintLog {
		dglogger.Infof(ctx, "call url: %s, cost: %v", request.URL.String(), cost)
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

func FillHeaders(request *http.Request, headers map[string]string) {
	if headers != nil && len(headers) > 0 {
		for k, v := range headers {
			request.Header[k] = []string{v}
		}
	}
}

func ExtractResponse(ctx *dgctx.DgContext, response *http.Response) (int, map[string][]string, []byte, error) {
	data, err := ReadResponse(response)
	if err != nil {
		dglogger.Errorf(ctx, "read response error, url: %s, err: %v", response.Request.URL, err)
	}

	if response.StatusCode >= http.StatusBadRequest {
		dglogger.Errorf(ctx, "request fail, url: %s，status code: %d", response.Request.URL, response.StatusCode)
	}

	if response.StatusCode >= http.StatusMultipleChoices {
		err = nil
	}

	return response.StatusCode, response.Header, data, err
}

func ReadResponse(resp *http.Response) ([]byte, error) {
	if resp == nil {
		return nil, nil
	}
	if resp.Body == nil {
		return nil, nil
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return io.ReadAll(resp.Body)
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

func ConvertResponse2Struct[T any](resp *http.Response) (*T, error) {
	bs, err := ReadResponse(resp)
	if err != nil {
		return nil, err
	}
	if len(bs) == 0 {
		return nil, nil
	}

	return utils.ConvertJsonBytesToBean[T](bs)
}
