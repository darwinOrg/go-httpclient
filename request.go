package dghttp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	nu "net/url"
	"sort"
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

func BuildRequest2CurlParts(request *http.Request) []string {
	parts := []string{"curl", "-X", bashEscape(request.Method)}

	if request.Method == http.MethodPost && request.Body != nil {
		body, _ := io.ReadAll(request.Body)
		if len(body) > 0 {
			SetRequestBody(request, body)
			bodyEscaped := bashEscape(string(body))
			parts = append(parts, "-d", bodyEscaped)
		}
	}

	var keys []string
	for k := range request.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		parts = append(parts, "-H", bashEscape(fmt.Sprintf("%s: %s", k, strings.Join(request.Header[k], " "))))
	}

	requestUrl := request.URL.String()
	parts = append(parts, bashEscape(requestUrl))

	return parts
}

func ConvertRequest2Curl(request *http.Request) string {
	return strings.Join(BuildRequest2CurlParts(request), " ")
}

func bashEscape(str string) string {
	return `'` + strings.Replace(str, `'`, `'\''`, -1) + `'`
}
