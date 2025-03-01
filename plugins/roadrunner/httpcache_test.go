package roadrunner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

type (
	next          struct{}
	configWrapper struct {
		*dummyConfiguration
	}
)

func (*configWrapper) Get(_ string) interface{} {
	var c map[string]interface{}
	_ = yaml.Unmarshal(dummyBadgerConfig, &c)

	return c
}

func (n *next) ServeHTTP(rw http.ResponseWriter, _ *http.Request) {
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write([]byte("Hello Roadrunner!"))
}

var nextFilter = &next{}

func prepare(endpoint string) (req *http.Request, res1 *httptest.ResponseRecorder, res2 *httptest.ResponseRecorder) {
	req = httptest.NewRequest(http.MethodGet, endpoint, nil)
	res1 = httptest.NewRecorder()
	res2 = httptest.NewRecorder()

	return
}

func Test_Plugin_Init(t *testing.T) {
	p := &Plugin{}

	if p.Init(&configWrapper{}, nil) != nil {
		t.Error("The Init method must not crash if a valid configuration is given.")
	}

	defer func() {
		if recover() == nil {
			t.Error("The Init method must crash if a nil configuration is given.")
		}
	}()
	err := p.Init(nil, nil)
	if err != nil {
		t.Error(err.Error())
	}
}

func Test_Plugin_Middleware(t *testing.T) {
	p := &Plugin{}
	_ = p.Init(&configWrapper{}, nil)
	handler := p.Middleware(nextFilter)
	req, res, res2 := prepare("/handled")
	handler.ServeHTTP(res, req)
	rs := res.Result()
	err := rs.Body.Close()
	if err != nil {
		t.Error("body close error")
	}
	if rs.Header.Get("Cache-Status") != "Souin; fwd=uri-miss; stored" {
		t.Error("The response must contain a Cache-Status header with the stored directive.")
	}
	handler.ServeHTTP(res2, req)
	rs = res2.Result()
	err = rs.Body.Close()
	if err != nil {
		t.Error("body close error")
	}
	if rs.Header.Get("Cache-Status") != "Souin; hit; ttl=4" {
		t.Error("The response must contain a Cache-Status header with the hit and ttl directives.")
	}
	if rs.Header.Get("Age") != "1" {
		t.Error("The response must contain a Age header with the value 1.")
	}
}

func Test_Plugin_Middleware_Stale(t *testing.T) {
	p := &Plugin{}
	_ = p.Init(&configWrapper{}, nil)
	handler := p.Middleware(nextFilter)

	var rs *http.Response
	common := func() {
		req, res, res2 := prepare("/stale-test")
		handler.ServeHTTP(res, req)
		rs := res.Result()
		err := rs.Body.Close()
		if err != nil {
			t.Error("body close error")
		}
		if rs.Header.Get("Cache-Status") != "Souin; fwd=uri-miss; stored" {
			t.Error("The response must contain a Cache-Status header with the stored directive.")
		}
		handler.ServeHTTP(res2, req)
		rs = res2.Result()
		err = rs.Body.Close()
		if err != nil {
			t.Error("body close error")
		}
		if rs.Header.Get("Cache-Status") != "Souin; hit; ttl=4" {
			t.Error("The response must contain a Cache-Status header with the hit directive.")
		}
		if rs.Header.Get("Age") != "1" {
			t.Error("The response must contain a Age header with the value 1.")
		}
	}

	common()
	req, _, _ := prepare("/stale-test")

	time.Sleep(5 * time.Second)
	res3 := httptest.NewRecorder()
	req.Header = http.Header{
		"Cache-Control": []string{"max-stale=4"},
	}
	handler.ServeHTTP(res3, req)
	rs = res3.Result()
	err := rs.Body.Close()
	if err != nil {
		t.Error("body close error")
	}
	if rs.Header.Get("Cache-Status") != "Souin; hit; ttl=-1; fwd=stale" {
		fmt.Println(rs.Header.Get("Cache-Status"))
		t.Error("The response must contain a Cache-Status header without the stored directive and with ttl=-1; fwd=stale.")
	}
	if rs.Header.Get("Age") != "6" {
		fmt.Println(rs.Header.Get("Age"))
		t.Error("The response must contain a Age header.")
	}

	common()
}

func Test_Plugin_Middleware_Excluded(t *testing.T) {
	p := &Plugin{}
	_ = p.Init(&configWrapper{}, nil)
	handler := p.Middleware(nextFilter)
	req, res, res2 := prepare("/excluded")
	handler.ServeHTTP(res, req)
	rs := res.Result()
	err := rs.Body.Close()
	if err != nil {
		t.Error("body close error")
	}
	if rs.Header.Get("Cache-Status") != "Souin; fwd=uri-miss" {
		t.Error("The response must contain a Cache-Status header without the stored directive and with the uri-miss only.")
	}
	handler.ServeHTTP(res2, req)
	rs = res2.Result()
	err = rs.Body.Close()
	if err != nil {
		t.Error("body close error")
	}
	if rs.Header.Get("Cache-Status") != "Souin; fwd=uri-miss" {
		t.Error("The response must contain a Cache-Status header without the stored directive and with the uri-miss only.")
	}
	if rs.Header.Get("Age") != "" {
		t.Error("The response must not contain a Age header.")
	}
}

func Test_Plugin_Middleware_Mutation(t *testing.T) {
	p := &Plugin{}
	_ = p.Init(&configWrapper{}, nil)
	handler := p.Middleware(nextFilter)
	req, res, res2 := prepare("/handled")
	req.Body = io.NopCloser(bytes.NewBuffer([]byte(`{"query":"mutation":{something mutated}}`)))
	handler.ServeHTTP(res, req)
	rs := res.Result()
	err := rs.Body.Close()
	if err != nil {
		t.Error("body close error")
	}
	if rs.Header.Get("Cache-Status") != "Souin; fwd=uri-miss" {
		t.Error("The response must contain a Cache-Status header without the stored directive and with the uri-miss only.")
	}
	req.Body = io.NopCloser(bytes.NewBuffer([]byte(`{"query":"mutation":{something mutated}}`)))
	handler.ServeHTTP(res2, req)
	rs = res2.Result()
	err = rs.Body.Close()
	if err != nil {
		t.Error("body close error")
	}
	if rs.Header.Get("Cache-Status") != "Souin; fwd=uri-miss" {
		t.Error("The response must contain a Cache-Status header without the stored directive and with the uri-miss only.")
	}
	if rs.Header.Get("Age") != "" {
		t.Error("The response must not contain a Age header.")
	}
}

func Test_Plugin_Middleware_API(t *testing.T) {
	p := &Plugin{}
	_ = p.Init(&configWrapper{}, nil)
	handler := p.Middleware(nextFilter)
	req, res, res2 := prepare("/httpcache_api/httpcache")
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("PURGE", "/httpcache_api/httpcache/.+", nil))
	handler.ServeHTTP(res, req)
	rs := res.Result()
	defer func() {
		err := rs.Body.Close()
		if err != nil {
			t.Error("body close error")
		}
	}()
	if rs.Header.Get("Content-Type") != "application/json" {
		t.Error("The response must be in JSON.")
	}
	b, _ := io.ReadAll(rs.Body)
	err := res.Result().Body.Close()
	if err != nil {
		t.Error("body close error")
	}
	if string(b) != "[]" {
		t.Error("The response body must be an empty array because no request has been stored")
	}
	req2 := httptest.NewRequest(http.MethodGet, "/handled", nil)
	handler.ServeHTTP(res2, req2)
	rs = res2.Result()
	err = rs.Body.Close()
	if err != nil {
		t.Error("body close error")
	}
	if rs.Header.Get("Cache-Status") != "Souin; fwd=uri-miss; stored" {
		t.Error("The response must contain a Cache-Status header with the stored directive.")
	}
	res3 := httptest.NewRecorder()
	handler.ServeHTTP(res3, req)
	rs = res3.Result()
	err = rs.Body.Close()
	if err != nil {
		t.Error("body close error")
	}
	if rs.Header.Get("Content-Type") != "application/json" {
		t.Error("The response must be in JSON.")
	}
	b, _ = io.ReadAll(rs.Body)
	err = rs.Body.Close()
	if err != nil {
		t.Error("body close error")
	}
	var payload []string
	_ = json.Unmarshal(b, &payload)
	if len(payload) != 2 {
		t.Error("The system must store 2 items, the fresh and the stale one")
	}
	if payload[0] != "GET-example.com-/handled" || payload[1] != "STALE_GET-example.com-/handled" {
		t.Error("The payload items mismatch from the expectations.")
	}
}
