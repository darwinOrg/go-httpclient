package dghttp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/darwinOrg/go-common/constants"
	dgctx "github.com/darwinOrg/go-common/context"
	dgsys "github.com/darwinOrg/go-common/sys"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var DefaultOtelHttpSpanNameFormatterOption = otelhttp.WithSpanNameFormatter(func(operation string, req *http.Request) string {
	return fmt.Sprintf("Call: %s %s", req.URL.String(), req.Method)
})

func NewOtelHttpTransport(rt http.RoundTripper) http.RoundTripper {
	return otelhttp.NewTransport(rt, DefaultOtelHttpSpanNameFormatterOption)
}

func NewOtelHttpTransportWithServiceName(rt http.RoundTripper, serviceName string) http.RoundTripper {
	return otelhttp.NewTransport(rt, otelhttp.WithSpanNameFormatter(func(operation string, req *http.Request) string {
		return fmt.Sprintf("%s: %s %s", serviceName, req.URL.Path, req.Method)
	}))
}

func SetSpanAttributesFromContext(ctx *dgctx.DgContext, rc context.Context, headers map[string]string) context.Context {
	var attrs []attribute.KeyValue

	profile := dgsys.GetProfile()
	if profile != "" {
		attrs = append(attrs, attribute.String(constants.Profile, profile))
	}
	if ctx.UserId > 0 {
		attrs = append(attrs, attribute.Int64(constants.UID, ctx.UserId))
	}
	if ctx.OpId > 0 {
		attrs = append(attrs, attribute.Int64(constants.OpId, ctx.OpId))
	}
	if ctx.RunAs > 0 {
		attrs = append(attrs, attribute.Int64(constants.RunAs, ctx.RunAs))
	}
	if ctx.Roles != "" {
		attrs = append(attrs, attribute.String(constants.Roles, ctx.Roles))
	}
	if ctx.BizTypes > 0 {
		attrs = append(attrs, attribute.Int(constants.BizTypes, ctx.BizTypes))
	}
	if ctx.GroupId > 0 {
		attrs = append(attrs, attribute.Int64(constants.GroupId, ctx.GroupId))
	}
	if ctx.Platform != "" {
		attrs = append(attrs, attribute.String(constants.Platform, ctx.Platform))
	}
	if ctx.CompanyId > 0 {
		attrs = append(attrs, attribute.Int64(constants.CompanyId, ctx.CompanyId))
	}
	if ctx.Product > 0 {
		attrs = append(attrs, attribute.Int(constants.Product, ctx.Product))
	}
	if len(ctx.Products) > 0 {
		attrs = append(attrs, attribute.IntSlice(constants.Products, ctx.Products))
	}
	if len(ctx.DepartmentIds) > 0 {
		attrs = append(attrs, attribute.Int64Slice(constants.Products, ctx.DepartmentIds))
	}

	if len(headers) > 0 {
		for k, v := range headers {
			attrs = append(attrs, attribute.String(k, v))
		}
	}

	if len(attrs) > 0 {
		span := trace.SpanFromContext(rc)
		span.SetAttributes(attrs...)
	}

	return rc
}
