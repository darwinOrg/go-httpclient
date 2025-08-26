package dghttp

import (
	"bytes"
	"net/http"
	nu "net/url"
	"strings"

	dgctx "github.com/darwinOrg/go-common/context"
	"github.com/darwinOrg/go-common/utils"
	dglogger "github.com/darwinOrg/go-logger"
	dgotel "github.com/darwinOrg/go-otel"
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

	var (
		request *http.Request
		err     error
	)
	if dgotel.Tracer != nil && hc.EnableTracer && ctx.GetInnerContext() != nil {
		c, span := dgotel.Tracer.Start(ctx.GetInnerContext(), "http_client")
		defer span.End()
		dgotel.SetSpanAttributesByMap(span, params)
		dgotel.SetSpanAttributesByMap(span, headers)
		request, err = http.NewRequestWithContext(c, http.MethodGet, url, nil)
	} else {
		request, err = http.NewRequest(http.MethodGet, url, nil)
	}
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

	var request *http.Request
	if dgotel.Tracer != nil && hc.EnableTracer && ctx.GetInnerContext() != nil {
		c, span := dgotel.Tracer.Start(ctx.GetInnerContext(), "http_client")
		defer span.End()
		dgotel.SetSpanAttributesByMap(span, map[string]string{"body": string(paramsBytes)})
		dgotel.SetSpanAttributesByMap(span, headers)
		request, err = http.NewRequestWithContext(c, http.MethodPost, url, bytes.NewBuffer(paramsBytes))
	} else {
		request, err = http.NewRequest(http.MethodPost, url, bytes.NewBuffer(paramsBytes))
	}
	if err != nil {
		dglogger.Errorf(ctx, "new request error, url: %s, params: %v, err: %v", url, params, err)
		return nil, err
	}

	request.Header.Set("Content-Type", jsonContentType)
	WriteHeaders(request, headers)
	WriteSseHeaders(request)

	return hc.DoRequestRaw(ctx, request)
}
