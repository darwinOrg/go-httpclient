package dghttp

import "testing"

// 回归：Http2Transport 必须提供 h2c（明文 HTTP/2）语义。
// Go 1.24+ 用 http.Protocols 替代废弃的 x/net/http2.Transport；
// 若误把 HTTP1 打开或未开 UnencryptedHTTP2，http:// 请求会回落到 HTTP/1.1，
// 依赖 h2c 的调用方（go-iflytek、atmosphere、galaxy-sdk 等）行为会改变。
func TestHttp2TransportUsesH2c(t *testing.T) {
	tr := Http2Transport // 适配后为 *http.Transport（旧版为 *http2.Transport）
	if tr == nil {
		t.Fatal("Http2Transport 为 nil")
	}
	if tr.Protocols == nil {
		t.Fatal("Http2Transport.Protocols 未设置，http:// 将走默认 HTTP/1.1")
	}
	if tr.Protocols.HTTP1() {
		t.Fatal("Protocols.HTTP1 已开启，h2c 会被 HTTP/1.1 抢占")
	}
	if !tr.Protocols.HTTP2() {
		t.Fatal("Protocols.HTTP2 未开启，https 无法协商 h2")
	}
	if !tr.Protocols.UnencryptedHTTP2() {
		t.Fatal("Protocols.UnencryptedHTTP2 未开启，失去 h2c 语义")
	}
}

// 回归：Client2 与默认客户端的协议契约。
func TestClient2AndDefaultUseH2cTransport(t *testing.T) {
	if Client2.HttpClient.Transport != Http2Transport {
		t.Fatal("Client2 未使用 Http2Transport")
	}
	// 不依赖 use_http11 环境变量的当前值，只校验 DefaultHttpClient 返回合法客户端。
	if DefaultHttpClient() == nil {
		t.Fatal("DefaultHttpClient 返回 nil")
	}
}
