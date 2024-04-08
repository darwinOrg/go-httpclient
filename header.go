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
	header[constants.Profile] = []string{dgsys.GetProfile()}
	header[constants.TraceId] = []string{ctx.TraceId}
	header[constants.UID] = []string{strconv.FormatInt(ctx.UserId, 10)}
	header[constants.OpId] = []string{strconv.FormatInt(ctx.OpId, 10)}
	header[constants.Roles] = []string{ctx.Roles}
	header[constants.BizTypes] = []string{strconv.Itoa(ctx.BizTypes)}
	header[constants.Platform] = []string{ctx.Platform}
	header[constants.Token] = []string{ctx.Token}
	header[constants.ShareToken] = []string{ctx.ShareToken}
	header[constants.RemoteIp] = []string{ctx.RemoteIp}
	header[constants.CompanyId] = []string{strconv.FormatInt(ctx.CompanyId, 10)}
	header[constants.Product] = []string{strconv.Itoa(ctx.Product)}
	header[constants.DepartmentIds] = []string{dgcoll.JoinInts(ctx.DepartmentIds, ",")}
}
