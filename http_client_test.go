package dghttp

import (
	dgctx "github.com/darwinOrg/go-common/context"
	dglogger "github.com/darwinOrg/go-logger"
	"net/http"
	"testing"
)

func TestDgHttpClient_DoGet(t *testing.T) {
	ctx := &dgctx.DgContext{TraceId: "123"}
	url := "http://localhost:8080/health"
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	resp, err := GlobalHttpClient.DoRequestRaw(ctx, req)
	if err != nil {
		dglogger.Infoln(ctx, err)
		return
	}
	dglogger.Infoln(ctx, resp)
}
