package dghttp

import (
	"bytes"
	"encoding/json"
	"net/http"

	dgctx "github.com/darwinOrg/go-common/context"
	dglogger "github.com/darwinOrg/go-logger"
)

func (hc *DgHttpClient) SseGet(ctx *dgctx.DgContext, url string, params map[string]string, headers map[string]string) (*http.Response, error) {
	url = AppendUrlParams(url, params)
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
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		dglogger.Errorf(ctx, "json marshal error, url: %s, params: %v, err: %v", url, params, err)
		return nil, err
	}

	var request *http.Request
	request, err = http.NewRequest(http.MethodPost, url, bytes.NewBuffer(paramsBytes))
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, params: %v, err: %v", url, params, err)
		return nil, err
	}

	request.Header.Set("Content-Type", jsonContentType)
	WriteHeaders(request, headers)
	WriteSseHeaders(request)

	return hc.DoRequestRaw(ctx, request)
}
