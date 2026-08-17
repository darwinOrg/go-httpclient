package dghttp

import (
	"io"
	"net/http"

	dgctx "github.com/darwinOrg/go-common/context"
	"github.com/darwinOrg/go-common/utils"
	dglogger "github.com/darwinOrg/go-logger"
)

func ExtractResponse(ctx *dgctx.DgContext, response *http.Response) (int, map[string][]string, []byte, error) {
	data, err := ReadResponse(response)
	if err != nil {
		dglogger.Errorf(ctx, "read response error, url: %s, err: %v", response.Request.URL, err)
	}

	if response.StatusCode >= http.StatusBadRequest {
		dglogger.Errorf(ctx, "request fail, url: %s，status code: %d", response.Request.URL, response.StatusCode)
	}

	if response.StatusCode >= http.StatusMultipleChoices {
		err = nil
	}

	return response.StatusCode, response.Header, data, err
}

func ReadResponse(resp *http.Response) ([]byte, error) {
	if resp == nil {
		return nil, nil
	}
	if resp.Body == nil {
		return nil, nil
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return io.ReadAll(resp.Body)
}

func ConvertResponse2Struct[T any](resp *http.Response) (*T, error) {
	bs, err := ReadResponse(resp)
	if err != nil {
		return nil, err
	}
	if len(bs) == 0 {
		return nil, nil
	}

	return utils.ConvertJsonBytesToBean[T](bs)
}
