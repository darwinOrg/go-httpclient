package dghttp

import (
	"io"
	"net/http"

	dgctx "github.com/darwinOrg/go-common/context"
)

func CopyRequest(ctx *dgctx.DgContext, rawReq *http.Request, newUrl string, body io.Reader) (*http.Request, error) {
	if newUrl == "" {
		newUrl = rawReq.URL.String()
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
