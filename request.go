package dghttp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	nu "net/url"
	"strings"

	dgctx "github.com/darwinOrg/go-common/context"
	"github.com/darwinOrg/go-common/utils"
)

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
	if len(params) == 0 || len(params) == 0 {
		return url
	}

	vs := nu.Values{}
	for k, v := range params {
		vs.Add(k, v)
	}
	url += utils.IfReturn(strings.Contains(url, "?"), "&", "?")
	url += vs.Encode()
	return url
}
