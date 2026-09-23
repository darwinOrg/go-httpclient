package dghttp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	dgctx "github.com/darwinOrg/go-common/context"
	dglogger "github.com/darwinOrg/go-logger"
)

var sseDataPrefixBytes = []byte("data:")

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

func HandleSseData(resp *http.Response, sleepTime time.Duration, maxTimes int, handler func(data []byte)) {
	defer func() { _ = resp.Body.Close() }()
	reader := bufio.NewReader(resp.Body)

	defer func() {
		if err := recover(); err != nil {
			log.Printf("HandleSseData panic: %v\n", err)
		}
	}()

	times := 0
	for times < maxTimes {
		rawLine, readErr := reader.ReadBytes('\n')
		if readErr == io.EOF {
			break
		}

		if len(rawLine) == 0 {
			time.Sleep(sleepTime)
			times++
			continue
		}

		if !bytes.HasPrefix(rawLine, sseDataPrefixBytes) {
			continue
		}

		handler(bytes.TrimRight(bytes.TrimPrefix(rawLine, sseDataPrefixBytes), "\r\n"))
		time.Sleep(sleepTime)
		times++
	}

	log.Printf("HandleSseData finished, total times: %d\n", times)
}
