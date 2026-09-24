package dghttp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	dgctx "github.com/darwinOrg/go-common/context"
	dglogger "github.com/darwinOrg/go-logger"
)

const (
	sseDataPrefix = "data:"
	sseDone       = "DONE"
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

func HandleSseData(resp *http.Response, handler func(data string)) error {
	defer func() { _ = resp.Body.Close() }()
	reader := bufio.NewReader(resp.Body)

	for {
		rawLine, readErr := reader.ReadString('\n')
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}

			return readErr
		}

		if !strings.HasPrefix(rawLine, sseDataPrefix) {
			continue
		}

		data := strings.TrimRight(strings.TrimPrefix(rawLine, sseDataPrefix), "\r\n")
		if data == "" {
			continue
		}

		if data == sseDone {
			return nil
		}

		handler(data)
	}
}
