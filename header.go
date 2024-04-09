package dghttp

import (
	dgcoll "github.com/darwinOrg/go-common/collection"
	"github.com/darwinOrg/go-common/constants"
	dgctx "github.com/darwinOrg/go-common/context"
	dgsys "github.com/darwinOrg/go-common/sys"
	"net/http"
	"strconv"
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
	if len(ctx.DepartmentIds) > 0 {
		header[constants.DepartmentIds] = []string{dgcoll.JoinInts(ctx.DepartmentIds, ",")}
	}
}
