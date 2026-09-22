package dghttp

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	dgctx "github.com/darwinOrg/go-common/context"
)

func ConvertJsonCurl(method, url string, params any, headers map[string]string) string {
	ctx := dgctx.SimpleDgContext()
	request, _ := BuildJsonRequest(ctx, method, url, params, headers)
	return ConvertRequest2Curl(request)
}

func ConvertRequest2Curl(request *http.Request) string {
	parts := []string{"curl", "-X", bashEscape(request.Method)}

	if request.Method == http.MethodPost && request.Body != nil {
		body, _ := io.ReadAll(request.Body)
		if len(body) > 0 {
			SetRequestBody(request, body)
			bodyEscaped := bashEscape(string(body))
			parts = append(parts, "-d", bodyEscaped)
		}
	}

	var keys []string
	for k := range request.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		parts = append(parts, "-H", bashEscape(fmt.Sprintf("%s: %s", k, strings.Join(request.Header[k], " "))))
	}

	requestUrl := request.URL.String()
	parts = append(parts, bashEscape(requestUrl))

	return strings.Join(parts, " ")
}

func bashEscape(str string) string {
	return `'` + strings.Replace(str, `'`, `'\''`, -1) + `'`
}
