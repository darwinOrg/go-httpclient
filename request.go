package dghttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	nu "net/url"
	"strings"

	dgctx "github.com/darwinOrg/go-common/context"
	"github.com/darwinOrg/go-common/utils"
	dglogger "github.com/darwinOrg/go-logger"
)

func BuildJsonRequest(ctx *dgctx.DgContext, method, url string, params any, headers map[string]string) (*http.Request, error) {
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
	request, err = http.NewRequest(method, url, bytes.NewBuffer(paramsBytes))
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, params: %v, err: %v", url, params, err)
		return nil, err
	}

	FillHeaders(request, headers)
	request.Header.Set(contentTypeHeader, jsonContentType)

	return request, nil
}

func CopyRequest(ctx *dgctx.DgContext, rawReq *http.Request, newUrl string, body io.Reader) (*http.Request, error) {
	if newUrl == "" {
		newUrl = GetFullURL(rawReq)
	}
	if body == nil {
		body = rawReq.Body
	}

	var (
		newReq *http.Request
		err    error
	)
	if ctx.GetInnerContext() != nil {
		newReq, err = http.NewRequestWithContext(ctx.GetInnerContext(), rawReq.Method, newUrl, body)
	} else {
		newReq, err = http.NewRequest(rawReq.Method, newUrl, body)
	}
	if err != nil {
		return nil, err
	}

	newReq.Header = rawReq.Header
	return newReq, nil
}

func GetFullURL(req *http.Request) string {
	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}
	// 如果服务在反向代理后面，优先从请求头获取
	if fwdProto := req.Header.Get("X-Forwarded-Proto"); fwdProto != "" {
		scheme = fwdProto
	}

	host := req.Host
	if host == "" {
		host = req.URL.Host
	}

	return fmt.Sprintf("%s://%s%s", scheme, host, req.URL.RequestURI())
}

func MustRequestBodyString(req *http.Request) string {
	if req.Method == http.MethodGet || req.Method == http.MethodHead {
		return ""
	}

	body, _ := io.ReadAll(req.Body)
	if len(body) > 0 {
		SetRequestBody(req, body)
		return string(body)
	}

	return ""
}

func SetRequestBody(req *http.Request, body []byte) {
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
}

func AppendUrlParams(url string, params map[string]string) string {
	if len(params) == 0 {
		return url
	}

	return url + utils.IfReturn(strings.Contains(url, "?"), "&", "?") + ConvertUrlParams(params)
}

func ConvertUrlParams(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}

	vs := nu.Values{}
	for k, v := range params {
		vs.Add(k, v)
	}

	return vs.Encode()
}
