package ion

import (
	"net/http"
	"net/http/httptest"
)

// Endpoints serves as an in-memory handler registry for mocking HTTP Endpoint[REQ, RES] in tests.
var Endpoints = handlers{}

type handlers map[string]func(*http.Request, *httptest.ResponseRecorder)

func (h handlers) Handler(fn func(*http.Request, *httptest.ResponseRecorder), hostnames ...string) {
	for i := range hostnames {
		h[hostnames[i]] = fn
	}
}

func (h handlers) handle(r *http.Request) (*http.Response, bool) {
	w := httptest.NewRecorder()
	if fn, ok := h[r.URL.Hostname()]; ok {
		fn(r, w)
		return w.Result(), true
	}
	w.WriteHeader(http.StatusNotImplemented)
	return w.Result(), false
}
