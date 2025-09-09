package dghttp

import (
	"net/http"
	"strconv"

	dgcoll "github.com/darwinOrg/go-common/collection"
	"github.com/darwinOrg/go-common/constants"
	dgctx "github.com/darwinOrg/go-common/context"
	dgsys "github.com/darwinOrg/go-common/sys"
)

func FillHeadersWithDgContext(ctx *dgctx.DgContext, header http.Header) {
	profile := dgsys.GetProfile()
	if profile != "" {
		header[constants.Profile] = []string{profile}
	}
	if ctx.TraceId != "" {
		header[constants.TraceId] = []string{ctx.TraceId}
	}
	if ctx.UserId > 0 {
		header[constants.UID] = []string{strconv.FormatInt(ctx.UserId, 10)}
	}
	if ctx.OpId > 0 {
		header[constants.OpId] = []string{strconv.FormatInt(ctx.OpId, 10)}
	}
	if ctx.Roles != "" {
		header[constants.Roles] = []string{ctx.Roles}
	}
	if ctx.BizTypes > 0 {
		header[constants.BizTypes] = []string{strconv.Itoa(ctx.BizTypes)}
	}
	if ctx.Platform != "" {
		header[constants.Platform] = []string{ctx.Platform}
	}
	if ctx.Token != "" {
		header[constants.Token] = []string{ctx.Token}
	}
	if ctx.ShareToken != "" {
		header[constants.ShareToken] = []string{ctx.ShareToken}
	}
	if ctx.RemoteIp != "" {
		header[constants.RemoteIp] = []string{ctx.RemoteIp}
	}
	if ctx.CompanyId != 0 {
		header[constants.CompanyId] = []string{strconv.FormatInt(ctx.CompanyId, 10)}
	}
	if ctx.Product > 0 {
		header[constants.Product] = []string{strconv.Itoa(ctx.Product)}
	}
	if len(ctx.Products) > 0 {
		header[constants.Products] = []string{dgcoll.JoinInts(ctx.Products, ",")}
	}
	if len(ctx.DepartmentIds) > 0 {
		header[constants.DepartmentIds] = []string{dgcoll.JoinInts(ctx.DepartmentIds, ",")}
	}
	if ctx.Source != "" {
		header[constants.Source] = []string{ctx.Source}
	}
	if ctx.Since != 0 {
		header[constants.Since] = []string{strconv.FormatInt(ctx.Since, 10)}
	}
}

func WriteHeaders(request *http.Request, headers map[string]string) {
	if headers != nil && len(headers) > 0 {
		for k, v := range headers {
			request.Header[k] = []string{v}
		}
	}
}

func WriteSseHeaders(request *http.Request) {
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("Connection", "keep-alive")
}
