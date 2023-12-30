package dghttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"github.com/darwinOrg/go-common/constants"
	dgctx "github.com/darwinOrg/go-common/context"
	"github.com/darwinOrg/go-common/result"
	dgsys "github.com/darwinOrg/go-common/sys"
	"github.com/darwinOrg/go-common/utils"
	dglogger "github.com/darwinOrg/go-logger"
	"github.com/darwinOrg/go-monitor"
	"golang.org/x/net/http2"
	"io"
	"net"
	"net/http"
	nu "net/url"
	"os"
	"strings"
	"time"
)

const (
	originalUrl                     = "originalUrl"
	jsonContentType                 = "application/json; charset=utf-8"
	formUrlEncodedContentType       = "application/x-www-form-urlencoded; charset=utf-8"
	DefaultTimeoutSeconds     int64 = 30
	UseHttp11                       = "use_http11"
	httpClientKey                   = "httpClient"
)

var (
	HttpTransport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		IdleConnTimeout: time.Duration(int64(time.Second) * DefaultTimeoutSeconds),
	}

	Http2Transport = &http2.Transport{
		// So http2.Transport doesn't complain the URL scheme isn't 'https'
		AllowHTTP: true,
		// Pretend we are dialing a TLS endpoint. (Note, we ignore the passed tls.Config)
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			return net.Dial(network, addr)
		},
	}

	GlobalHttpClient = DefaultHttpClient()
	Client11         = NewHttpClient(true, DefaultTimeoutSeconds)
	Client2          = NewHttpClient(false, DefaultTimeoutSeconds)
)

type DgHttpClient struct {
	HttpClient *http.Client
	UseMonitor bool
}

func DefaultHttpClient() *DgHttpClient {
	useHttp11, ok := os.LookupEnv(UseHttp11)
	return NewHttpClient(ok && useHttp11 == "true", DefaultTimeoutSeconds)
}

func NewHttpClient(useHttp11 bool, timeoutSeconds int64) *DgHttpClient {
	userMonitor := true

	profile := dgsys.GetProfile()
	if profile == "local" || profile == "" {
		userMonitor = false
	}

	httpClient := &http.Client{
		Timeout: time.Duration(int64(time.Second) * timeoutSeconds),
	}

	if useHttp11 {
		httpClient.Transport = HttpTransport
	} else {
		httpClient.Transport = Http2Transport
	}

	return &DgHttpClient{
		HttpClient: httpClient,
		UseMonitor: userMonitor,
	}
}

func (hc *DgHttpClient) DoGet(ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) ([]byte, error) {
	ctx.SetExtraKeyValue(originalUrl, url)
	if len(params) > 0 {
		if params != nil && len(params) > 0 {
			vs := nu.Values{}
			for k, v := range params {
				vs.Add(k, v)
			}
			if strings.Contains(url, "?") {
				url += "&"
			} else {
				url += "?"
			}
			url += vs.Encode()
		}
	}

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, err: %v", url, err)
		return nil, err
	}

	return hc.doRequest(ctx, request, headers)
}

func (hc *DgHttpClient) DoPostJson(ctx *dgctx.DgContext, url string, params any, headers map[string]string) ([]byte, error) {
	ctx.SetExtraKeyValue(originalUrl, url)
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		dglogger.Errorf(ctx, "json marshal error, url: %s, params: %v, err: %v", url, params, err)
		return nil, err
	}
	dglogger.Infof(ctx, "post request, url: %s, params: %v", url, string(paramsBytes))

	request, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(paramsBytes))
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, params: %v, err: %v", url, params, err)
		return nil, err
	}
	request.Header.Set("Content-Type", jsonContentType)
	//request.Header.Set("Content-Length", fmt.Sprintf("%d", len(paramsBytes)))

	return hc.doRequest(ctx, request, headers)
}

func (hc *DgHttpClient) DoPostFormUrlEncoded(ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) ([]byte, error) {
	ctx.SetExtraKeyValue(originalUrl, url)
	var paramsArr []string
	for k, v := range params {
		paramsArr = append(paramsArr, k+"="+v)
	}
	paramsStr := strings.Join(paramsArr, "&")
	dglogger.Infof(ctx, "post request, url: %s, params: %s", url, paramsStr)

	request, err := http.NewRequest(http.MethodPost, url, strings.NewReader(paramsStr))
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, params: %v, err: %v", url, params, err)
		return nil, err
	}
	request.Header.Set("Content-Type", formUrlEncodedContentType)

	return hc.doRequest(ctx, request, headers)
}

func (hc *DgHttpClient) DoUploadBodyFromLocalFile(ctx *dgctx.DgContext, method string, url string, filePath string) ([]byte, error) {
	fh, err := os.Open(filePath)
	if err != nil {
		dglogger.Errorf(ctx, "error opening file: %s", filePath)
		return nil, errors.New("error opening file")
	}
	defer fh.Close()

	return hc.DoUploadBody(ctx, method, url, fh)
}

func (hc *DgHttpClient) DoUploadBody(ctx *dgctx.DgContext, method string, url string, body io.Reader) ([]byte, error) {
	ctx.SetExtraKeyValue(originalUrl, url)
	dglogger.Infof(ctx, "upload, url: %s", url)

	request, err := http.NewRequest(method, url, body)
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, err: %v", url, err)
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return hc.doRequest(ctx, request, nil)
}

func (hc *DgHttpClient) doRequest(ctx *dgctx.DgContext, request *http.Request, headers map[string]string) ([]byte, error) {
	if headers != nil && len(headers) > 0 {
		for k, v := range headers {
			request.Header[k] = []string{v}
		}
	}
	_, _, body, err := hc.DoRequest(ctx, request)
	return body, err
}

func (hc *DgHttpClient) DoRequest(ctx *dgctx.DgContext, request *http.Request) (int, map[string][]string, []byte, error) {
	response, err := hc.DoRequestRaw(ctx, request)
	if err != nil {
		return http.StatusInternalServerError, nil, nil, err
	}

	defer func(b io.ReadCloser) {
		err := b.Close()
		if err != nil {
			dglogger.Errorf(ctx, "close response body error, url: %s, err: %v", request.URL.String(), err)
		}
	}(response.Body)

	if response.StatusCode >= 400 {
		return response.StatusCode, response.Header, nil, errors.New("request fail: " + response.Status)
	}

	if response.StatusCode >= 300 {
		return response.StatusCode, response.Header, nil, nil
	}

	data, err := io.ReadAll(response.Body)
	return response.StatusCode, response.Header, data, err
}

func (hc *DgHttpClient) DoRequestRaw(ctx *dgctx.DgContext, request *http.Request) (*http.Response, error) {
	start := time.Now().UnixMilli()
	if hc.UseMonitor {
		if ctx.GetExtraValue(originalUrl) != nil {
			monitor.HttpClientCounter(ctx.GetExtraValue(originalUrl).(string))
		} else {
			monitor.HttpClientCounter(request.URL.String())
		}
	}

	request.Header[constants.TraceId] = []string{ctx.TraceId}
	request.Header[constants.Profile] = []string{dgsys.GetProfile()}
	response, err := hc.HttpClient.Do(request)

	cost := time.Now().UnixMilli() - start
	if hc.UseMonitor {
		e := "false"
		if err != nil {
			e = "true"
		}
		if ctx.GetExtraValue(originalUrl) != nil {
			monitor.HttpClientDuration(ctx.GetExtraValue(originalUrl).(string), e, cost)
		} else {
			monitor.HttpClientDuration(request.URL.String(), e, cost)
		}
	}
	if err != nil {
		dglogger.Infof(ctx, "call url: %s, cost: %d ms, err: %v", request.URL.String(), cost, err)
		return response, err
	} else {
		dglogger.Infof(ctx, "call url: %s, cost: %d ms", request.URL.String(), cost)
	}

	return response, err
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
