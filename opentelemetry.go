package dghttp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/darwinOrg/go-common/constants"
	dgctx "github.com/darwinOrg/go-common/context"
	dgsys "github.com/darwinOrg/go-common/sys"
	"github.com/spf13/cast"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/baggage"
)

var OtelHttpSpanNameFormatterOption = otelhttp.WithSpanNameFormatter(func(operation string, req *http.Request) string {
	return fmt.Sprintf("Call: %s %s", req.URL.String(), req.Method)
})

func ContextWithBaggage(ctx *dgctx.DgContext, rc context.Context) context.Context {
	var members []baggage.Member
	profile := dgsys.GetProfile()
	if profile != "" {
		members = append(members, newBaggageMember(constants.Profile, profile))
	}
	if ctx.UserId > 0 {
		members = append(members, newBaggageMember(constants.UID, ctx.UserId))
	}
	if ctx.OpId > 0 {
		members = append(members, newBaggageMember(constants.OpId, ctx.OpId))
	}
	if ctx.RunAs > 0 {
		members = append(members, newBaggageMember(constants.RunAs, ctx.RunAs))
	}
	if ctx.Roles != "" {
		members = append(members, newBaggageMember(constants.Roles, ctx.Roles))
	}
	if ctx.BizTypes > 0 {
		members = append(members, newBaggageMember(constants.BizTypes, ctx.BizTypes))
	}
	if ctx.GroupId > 0 {
		members = append(members, newBaggageMember(constants.GroupId, ctx.GroupId))
	}
	if ctx.Platform != "" {
		members = append(members, newBaggageMember(constants.Platform, ctx.Platform))
	}
	if ctx.CompanyId > 0 {
		members = append(members, newBaggageMember(constants.CompanyId, ctx.CompanyId))
	}
	if ctx.Product > 0 {
		members = append(members, newBaggageMember(constants.Product, ctx.Product))
	}
	if len(ctx.Products) > 0 {
		members = append(members, newBaggageMember(constants.Products, ctx.Products))
	}
	if len(ctx.DepartmentIds) > 0 {
		members = append(members, newBaggageMember(constants.Products, ctx.DepartmentIds))
	}

	if len(members) > 0 {
		bag, _ := baggage.New(members...)
		return baggage.ContextWithBaggage(rc, bag)
	}

	return rc
}

func newBaggageMember(key string, value any) baggage.Member {
	member, _ := baggage.NewMember(key, cast.ToString(value))
	return member
}
