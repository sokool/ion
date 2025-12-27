package ion

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"time"

	"golang.org/x/exp/maps"
)

type Endpoint[REQ, RES any] struct {
	name    string
	path    string
	method  string
	body    REQ
	headers map[string]string
	params  values
	domain  *API
	key     string
	cache   time.Duration
	context context.Context
	limiter Limiter
	lock    bool
	log     *Logger
}

func NewEndpoint[REQ, RES any](url string, args ...any) Endpoint[REQ, RES] {
	a, err := APIFromURL(url, args...)
	if err != nil {
		Exit("Rest: %s", err)
	}
	u := a.URL
	return Endpoint[REQ, RES]{
		path:   u.Path,
		method: "GET",
		domain: &API{URL: u},
		params: u.URL.Query(),
		log:    NewLogger(""),
		name:   u.Hostname(),
	}
}

func JSONEndpoint(url string, args ...any) Endpoint[JSON, JSON] {
	return NewEndpoint[JSON, JSON](url, args...)
}

func APIEndpoint(d *API, path string, args ...any) Endpoint[Meta, JSON] {
	if d == nil {
		d = &API{}
	}

	if len(args) > 0 {
		path = fmt.Sprintf(path, args...)
	}
	e := Endpoint[Meta, JSON]{
		path:    path,
		method:  "GET",
		domain:  d,
		headers: make(map[string]string),
		params:  make(values),
		log:     NewLogger(""),
	}
	if d.Headers != nil {
		maps.Copy(e.headers, d.Headers)
	}
	return e
}

func (e Endpoint[REQ, RES]) Name(n string, args ...any) Endpoint[REQ, RES] {
	e.name = fmt.Sprintf(e.domain.Name+":"+n, args...)
	return e
}

func (e Endpoint[REQ, RES]) Method(n string) Endpoint[REQ, RES] {
	e.method = n
	return e
}

func (e Endpoint[REQ, RES]) Header(n, v string, args ...any) Endpoint[REQ, RES] {
	if e.headers == nil {
		e.headers = make(map[string]string)
	}
	e.headers[n] = fmt.Sprintf(fmt.Sprintf("%s", v), args...)
	return e
}

func (e Endpoint[REQ, RES]) Query(name, value string) Endpoint[REQ, RES] {
	e.params.Set(name, value)
	return e
}

func (e Endpoint[REQ, RES]) Limit(rps float64) Endpoint[REQ, RES] {
	e.limiter = NewLimiter(rps)
	return e
}

func (e Endpoint[REQ, RES]) Body(in REQ) Endpoint[REQ, RES] {
	e.body = in
	return e
}

func (e Endpoint[REQ, RES]) Get() (RES, error) {
	var r REQ
	return e.Method("GET").execute(r)
}

func (e Endpoint[REQ, RES]) Post(in REQ) (RES, error) {
	return e.Method("POST").execute(in)
}

func (e Endpoint[REQ, RES]) Execute() (RES, error) {
	return e.execute(e.body)
}

func (e Endpoint[REQ, RES]) String() string {
	var h string
	for n, v := range e.headers {
		h += fmt.Sprintf("%s: %s\n", n, v)
	}

	return fmt.Sprintf(`%s %s HTTP/1.1
Host: %s
%s`,
		e.method, e.path+"?"+e.params.Encode(), e.domain.URL.Format("scheme://host"), h)
}

func (e Endpoint[REQ, RES]) Errors(fn func(*http.Request, *http.Response, any) error) Endpoint[REQ, RES] {
	e.domain.Errors = fn
	return e
}

// Context sets a custom context for the endpoint execution.
//
// The provided `ctx` will be used during the endpoint's lifecycle, allowing
// you to control timeouts, cancellations, and carry metadata across chained
// operations.
//
// This is especially useful for propagating deadlines or tracing information
// in distributed systems.
func (e Endpoint[REQ, RES]) Context(ctx context.Context) Endpoint[REQ, RES] {
	e.context = ctx
	return e
}

// Cache enables response caching for the endpoint.
//
// When set, the endpoint response is cached for the given duration `d`.
// The cache key is automatically derived from the HTTP request data,
// including method, headers, query parameters, URL, and request body.
//
// Optionally, a static `name` can be provided to override the automatic
// key generation. When a `name` is given, it is used directly as the cache
// key instead of fingerprinting the request.
//
// This is useful for scenarios like static config endpoints or repeatable
// expensive computations.
func (e Endpoint[REQ, RES]) Cache(d time.Duration, name ...string) Endpoint[REQ, RES] {
	for i := range name {
		e.key += name[i]
	}
	e.cache = d
	return e
}

// Lock enables distributed locking for the endpoint.
//
// When enabled, Lock prevents concurrent invocations of the same endpoint
// with identical request parameters (headers, query, body) across multiple
// instances—whether they run in the same process or on different machines.
//
// This is particularly useful for deduplication, throttling race-prone workflows,
// or ensuring expensive operations don’t execute twice simultaneously.
//
// Note: Locking granularity is based on the full request fingerprint.
func (e Endpoint[REQ, RES]) Lock(enable bool) Endpoint[REQ, RES] {
	e.lock = enable
	return e
}

func (e Endpoint[REQ, RES]) wait(ctx context.Context) error {
	if e.limiter != nil {
		return e.limiter.Check(ctx, UUID(e.String()))
	}
	if e.domain != nil && e.domain.limiter != nil {
		return e.domain.limiter.Check(ctx, e.domain.Name)
	}
	return nil
}

func (e Endpoint[REQ, RES]) reader(contentType string, r REQ) (*strings.Reader, error) {
	v := any(r)
	switch contentType {
	case "application/x-www-form-urlencoded":
		switch v := v.(type) {
		case string:
			return strings.NewReader(fmt.Sprintf("%s", v)), nil
		}
		v, err := newValues(v)
		if err != nil {
			return nil, err
		}
		return strings.NewReader(v.Encode()), nil
	case "application/json":
		bfr := &bytes.Buffer{}
		if err := json.NewEncoder(bfr).Encode(v); err != nil {
			return nil, err
		}
		return strings.NewReader(bfr.String()), nil
	case "text/plain":
		return strings.NewReader(fmt.Sprintf("%v", v)), nil
	default:
		return strings.NewReader(""), nil
	}
}

func (e Endpoint[REQ, RES]) execute(in REQ) (RES, error) {
	var out RES
	if e.domain.URL == nil {
		return out, Errorf("domain url not found")
	}
	tag := e.tag()
	if e.headers == nil {
		e.headers = make(map[string]string)
	}
	if _, found := e.headers["Content-Type"]; !found && !isEmpty(in) {
		e.headers["Content-Type"] = "application/json"
	}

	rdr, err := e.reader(e.headers["Content-Type"], in)
	if err != nil {
		return out, err
	}
	url := fmt.Sprintf("%s%s", e.domain.URL.Format("scheme://host:port"), e.path)
	if s := e.params.Encode(); s != "" {
		url += "?" + s
	}
	cx := e.context
	if e.context == nil {
		cx = ctx
	}
	req, err := http.NewRequestWithContext(cx, e.method, url, rdr)
	if err != nil {
		return out, err
	}
	cx = req.Context()
	for n, v := range e.headers {
		req.Header[n] = []string{v}
	}

	var b []byte
	key, err := e.hash(req)
	if err != nil {
		return out, err
	}
	if e.lock {
		mu := NewLocker(cx, key)
		mu.Lock()
		defer mu.Unlock()
	}
	msg := fmt.Sprintf(tag+" %s:%s", e.method, e.path)
	code := ""
	if b = e.domain.get(cx, key, e.cache); b == nil {
		if err = e.wait(cx); err != nil {
			return out, err
		}
		now := time.Now()
		res, err := e.domain.run(req)
		if err != nil {
			return out, err
		}
		if code = res.Status; res.StatusCode >= 400 {
			body, _ := io.ReadAll(res.Body)
			defer res.Body.Close()
			if len(body) == 0 {
				body = []byte(code)
			}
			res.Body = io.NopCloser(bytes.NewBuffer(body))

			if e.domain.Errors != nil {
				err = e.domain.Errors(req, res, in)
			} else {
				err = Errorf("%s: %s", res.Status, string(body))
			}
			return out, err
		}
		b, _ = io.ReadAll(res.Body)
		if n := e.domain.set(key, b, e.cache); n > 0 {
			code = "200 Cached"
		}
		ins := float64(rdr.Size()) / 1024
		ous := float64(len(b)) / 1024
		Metrics.Percentile(`rest_in_seconds{domain=%q,method=%q,path=%q}`,
			time.Since(now).Seconds(), e.domain.Name, req.Method, req.URL.Path)
		e.log.Trace(2).Debugf(msg+" [%s] in|out: %.2f|%.2fkB in %s",
			code, ins, ous, time.Since(now))

	}

	if len(b) != 0 {
		if err = json.Unmarshal(b, &out); err != nil {
			e.log.Errorf(msg)
			return out, err
		}
	}
	return out, nil
}

func (e Endpoint[REQ, RES]) hash(r *http.Request) (string, error) {
	hash := md5.New()
	key := fmt.Sprintf("%s\n%s\n%s\n", r.Method, r.URL, e.key)
	if e.key == "" {
		hk := make([]string, 0, len(r.Header))
		for k := range r.Header {
			hk = append(hk, k)
		}
		sort.Strings(hk)
		for _, k := range hk {
			key += fmt.Sprintf("%s: %s\n", k, strings.Join(r.Header[k], ", "))
		}

		// If there's a body, add it to the hash
		if r.Body != nil {
			bodyCopy, err := io.ReadAll(r.Body)
			if err != nil {
				return "", err
			}
			defer r.Body.Close()
			key += fmt.Sprintf("%s\n", string(bodyCopy))
			// Recreate the Body for further reads by other handlers
			r.Body = io.NopCloser(strings.NewReader(string(bodyCopy)))
		}

	}
	if _, err := hash.Write([]byte(key)); err != nil {
		return "", err
	}
	// Return the MD5 hash as a hex string
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (e Endpoint[REQ, RES]) tag() string {
	n := e.domain.URL.Hostname()
	if e.domain.Name != "" {
		n = e.domain.Name
	}
	if e.name != "" {
		n = e.name
	}
	return fmt.Sprintf("%s", n)
}

func isEmpty[T any](value T) bool {
	v := reflect.ValueOf(value)
	if (v.Kind() == reflect.Ptr || v.Kind() == reflect.Slice || v.Kind() == reflect.Map ||
		v.Kind() == reflect.Func || v.Kind() == reflect.Chan || v.Kind() == reflect.Interface) &&
		v.IsNil() {
		return true
	}
	return v.IsZero()
}

type values = url.Values

// ConvertStructToURLValues converts a struct into url.Values
func newValues(input any) (url.Values, error) {
	values := url.Values{}

	// Reflect the input to inspect its type
	v := reflect.ValueOf(input)
	t := reflect.TypeOf(input)

	switch v.Kind() {
	case reflect.Struct:
		// Handle struct
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			tag := field.Tag.Get("form")
			if tag == "" {
				tag = strings.ToLower(field.Name) // Fallback to lowercase field name
			}
			values.Set(tag, fmt.Sprintf("%v", v.Field(i).Interface()))
		}
	case reflect.Map:
		// Handle map
		for _, key := range v.MapKeys() {
			// Ensure the key is a string
			if key.Kind() != reflect.String {
				return nil, fmt.Errorf("map keys must be strings")
			}
			// Add key-value pairs to url.Values
			value := v.MapIndex(key)
			values.Set(fmt.Sprintf("%v", key), fmt.Sprintf("%v", value.Interface()))
		}
	default:
		return nil, fmt.Errorf("unsupported type: %T", input)
	}

	return values, nil
}
