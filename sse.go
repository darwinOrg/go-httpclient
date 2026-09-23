package dghttp

import (
	"bufio"
	"bytes"
	"context"
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

func HandleSseData(resp *http.Response, maxTimes int, readTimeout, sleepTime time.Duration, handler func(data []byte)) {
	defer func() { _ = resp.Body.Close() }()

	defer func() {
		if err := recover(); err != nil {
			log.Printf("HandleSseData panic: %v\n", err)
		}
	}()

	scanner := bufio.NewScanner(resp.Body)
	// 设置最大缓冲区大小，防止单行数据过大导致崩溃（SSE 数据有时会很长）
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	times := 0
	for times < maxTimes {
		// 1. 在循环内部创建带有超时的 Context，确保每次读取都有独立的超时时间
		ctx, cancel := context.WithTimeout(context.Background(), readTimeout)

		// 2. 使用 channel 和 select 来实现带有超时的读取
		scanChan := make(chan bool, 1)
		go func() {
			defer cancel() // 读取完成后取消 context，释放资源
			scanChan <- scanner.Scan()
		}()

		select {
		case <-ctx.Done():
			// 读取超时，说明服务端可能卡住了，直接退出循环
			log.Printf("HandleSseData read timeout, exiting. Total times: %d\n", times)
			return
		case success := <-scanChan:
			if !success {
				// 读取结束或发生错误
				if err := scanner.Err(); err != nil && err != io.EOF {
					log.Printf("HandleSseData scanner error: %v\n", err)
				}
				return
			}
		}

		rawLine := scanner.Bytes()

		// 3. 处理空行（SSE 协议中通常用空行表示事件结束）
		if len(rawLine) == 0 {
			time.Sleep(sleepTime)
			times++
			continue
		}

		// 4. 处理 data 前缀
		if !bytes.HasPrefix(rawLine, sseDataPrefixBytes) {
			// 如果不是 data: 开头，可能是 comment 或 event 类型，根据需求决定是否跳过
			continue
		}

		// 提取数据并去除右侧的换行符/空白符
		data := bytes.TrimRight(bytes.TrimPrefix(rawLine, sseDataPrefixBytes), "\r\n")
		if len(data) > 0 {
			handler(data)
		}

		time.Sleep(sleepTime)
		times++
	}

	log.Printf("HandleSseData finished, total times: %d\n", times)
}
