package dghttp

import (
	dgctx "github.com/darwinOrg/go-common/context"
	dglogger "github.com/darwinOrg/go-logger"
	"testing"
)

func TestDgHttpClient_DoGet(t *testing.T) {
	ctx := &dgctx.DgContext{TraceId: "123"}
	url := "https://e.globalpand.cn/media/api/video/v1/4580093?definition=SD&format=mp4&expiration=1696839872182&accessKey=2cc50b085f5d65bcf6061e03aaabd454&signature=ef68e2dedd16b2b34d27eba6abdc3b8d8e3efd94&token=eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJwbHQiOiJCX1dFQl9QQyIsImlkIjoxMTg1MywiZXhwIjoxNjk5NDI4MDA4fQ.LFZ8tgmBMHiSEPAggIIf8HNi746mOMB9ByTOy0g0NOI"
	body, err := GlobalHttpClient.DoGet(ctx, url, map[string]string{}, make(map[string]string))
	if err != nil {
		dglogger.Infoln(ctx, err)
		return
	}
	dglogger.Infoln(ctx, body)
}
