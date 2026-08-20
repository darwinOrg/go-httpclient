package dghttp

import (
	"fmt"
	"io"
	"net/http"

	dgctx "github.com/darwinOrg/go-common/context"
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

	return fmt.Sprintf("%s://%s%s", scheme, host, req.URL.String())
}
