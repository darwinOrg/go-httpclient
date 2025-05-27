package dghttp

import (
	"bytes"
	dgctx "github.com/darwinOrg/go-common/context"
	"github.com/darwinOrg/go-common/utils"
	dglogger "github.com/darwinOrg/go-logger"
	"net/http"
	nu "net/url"
	"strings"
)

func (hc *DgHttpClient) SseGet(ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*http.Response, error) {
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

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, err: %v", url, err)
		return nil, err
	}
	WriteHeaders(request, headers)
	WriteSseHeaders(request)

	return hc.DoRequestRaw(ctx, request)
}

func (hc *DgHttpClient) SsePostJson(ctx *dgctx.DgContext, url string, params any, headers map[string]string) (*http.Response, error) {
	ctx.SetExtraKeyValue(originalUrl, url)
	paramsBytes, err := dglogger.Json(params)
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
	WriteHeaders(request, headers)
	WriteSseHeaders(request)

	return hc.DoRequestRaw(ctx, request)
}
