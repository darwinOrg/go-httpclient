package dghttp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	dgctx "github.com/darwinOrg/go-common/context"
	dglogger "github.com/darwinOrg/go-logger"
)

var sseDataPrefixBytes = []byte("data:")

const sseDefaultSleepTime = time.Millisecond * 10

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

func HandleSseData(resp *http.Response, handler func(data []byte)) {
	defer func() { _ = resp.Body.Close() }()
	reader := bufio.NewReader(resp.Body)

	for {
		rawLine, readErr := reader.ReadBytes('\n')
		if readErr == io.EOF {
			break
		}

		if !bytes.HasPrefix(rawLine, sseDataPrefixBytes) {
			continue
		}

		handler(bytes.TrimPrefix(rawLine, sseDataPrefixBytes))
		time.Sleep(sseDefaultSleepTime)
	}
}
