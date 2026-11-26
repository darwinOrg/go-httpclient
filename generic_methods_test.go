package dghttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	dgctx "github.com/darwinOrg/go-common/context"
	"github.com/darwinOrg/go-common/result"
)

// 验证 *DgHttpClient 自有泛型方法与包级委托包装返回一致结果。
func TestGenericMethods(t *testing.T) {
	const body = `{"success":true,"code":0,"message":"ok","messageCode":"OK","data":{"name":"alice","age":30}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	ctx := &dgctx.DgContext{TraceId: "generic-method-test"}
	hc := NewHttpClient(nil, 5)

	t.Run("method DoGetToResult", func(t *testing.T) {
		rt, err := hc.DoGetToResult[profile](ctx, srv.URL, nil, nil)
		if err != nil {
			t.Fatalf("DoGetToResult: %v", err)
		}
		if !rt.Success || rt.Data.Name != "alice" || rt.Data.Age != 30 {
			t.Fatalf("unexpected result: %+v", rt)
		}
	})

	t.Run("method DoGetToResultML", func(t *testing.T) {
		rt, err := hc.DoGetToResultML[profile](ctx, srv.URL, nil, nil)
		if err != nil {
			t.Fatalf("DoGetToResultML: %v", err)
		}
		if !rt.Success || rt.MessageCode != "OK" || rt.Data.Name != "alice" {
			t.Fatalf("unexpected result: %+v", rt)
		}
	})

	t.Run("method DoGetToResult explicit type argument", func(t *testing.T) {
		// T 仅出现在返回类型，无法从参数推断，必须显式写 [T]；
		// 这与包级泛型函数行为一致。
		var zero *result.Result[profile]
		zero, err := hc.DoGetToResult[profile](ctx, srv.URL, nil, nil)
		if err != nil {
			t.Fatalf("DoGetToResult: %v", err)
		}
		if zero == nil || zero.Data.Name != "alice" {
			t.Fatalf("unexpected result: %+v", zero)
		}
	})

	t.Run("method DoPostJsonToResult", func(t *testing.T) {
		rt, err := hc.DoPostJsonToResult[profile](ctx, srv.URL, map[string]string{"q": "x"}, nil)
		if err != nil {
			t.Fatalf("DoPostJsonToResult: %v", err)
		}
		if !rt.Success || rt.Data.Age != 30 {
			t.Fatalf("unexpected result: %+v", rt)
		}
	})

	t.Run("package wrapper delegates to ctx client", func(t *testing.T) {
		SetHttpClient(ctx, hc)
		rt, err := DoGetToResult[profile](ctx, srv.URL, nil, nil)
		if err != nil {
			t.Fatalf("package DoGetToResult: %v", err)
		}
		if !rt.Success || rt.Data.Name != "alice" {
			t.Fatalf("unexpected result: %+v", rt)
		}
	})
}

type profile struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}
