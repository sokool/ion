package ion

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type API struct {
	URL           *URL
	Name          string
	Authorization func(*http.Request) (string, time.Time, error)

	OnRequest func(*http.Request) bool

	// Headers is the default headers for all requests.
	Headers map[string]string

	// Proxy is the URL of the proxy server to use.
	Proxy string

	// MaxRequestsPerSecond is the maximum number of requests per second.
	// Every Endpoint generated from Endpoint method will be rate limited by this value.
	MaxRequestsPerSecond float64

	// Cache time that each request and response will be kept for given duration.
	// Set to 0 to disable caching.
	Cache time.Duration

	// Errors defines a custom error handler for HTTP responses with status code >= 400.
	//
	// The function receives the HTTP status code and response body as []byte input,
	// and returns an `error` which will be propagated by the Endpoint.Execute method.
	//
	// This allows customization of error formatting, logging, or mapping specific
	// HTTP errors to domain-specific ones.
	Errors  func(*http.Request, *http.Response, any) error
	mu      sync.Mutex
	limiter Limiter
	client  *http.Client
	log     *Logger
}

func NewAPI(osVarName string, required ...bool) (_ *API, err error) {
	s, ok := os.LookupEnv(osVarName)
	defer func() {
		if len(required) > 0 && required[0] && err != nil {
			log_.Errorf("%s", err)
			os.Exit(1)
		}
	}()
	if !ok && InUnitTests() {
		return &API{URL: MustURL("https://" + osVarName)}, nil
	}
	if !ok {
		return nil, Errorf("%s os variable is missing", osVarName)
	}
	if s == "" {
		return nil, Errorf("%s os variable is empty", osVarName)
	}

	return APIFromURL(s)
}

func APIFromURL(url string, args ...any) (*API, error) {
	if len(args) > 0 {
		url = fmt.Sprintf(url, args...)
	}
	u, err := ParseURL(url, "scheme", "host")
	if err != nil {
		return nil, Errorf("%s invalid format", url)
	}
	d := API{
		URL:     u,
		Headers: make(map[string]string),
		// Transport: sentryTransport, todo
	}
	for name, value := range u.URL.Query() {
		if name == "Cache" {
			if d.Cache, err = time.ParseDuration(value[0]); err != nil {
				return nil, Errorf("%s Cache query param must be string of time.Duration, %s given", u.Host, value[0])
			}
			continue
		}
		if name == "Name" {
			d.Name = value[0]
			continue
		}
		if name == "MaxRequestsPerSecond" {
			if d.MaxRequestsPerSecond, err = strconv.ParseFloat(value[0], 64); err != nil {
				return nil, Errorf("%s MaxRequestsPerSecond query param must be a number, %s given", u.Host, value[0])
			}
			d.limiter = NewLimiter(d.MaxRequestsPerSecond)
			continue
		}
		if n := strings.Index(name, "Header."); n != -1 {
			d.Headers[name[n+7:]] = value[0]
		}
	}
	if n := strings.ToLower(u.Username()); n != "" {
		switch {
		case n == "bearer":
			d.Header("Authorization", "Bearer %s", u.Password())
		case strings.HasPrefix(n, "x-"):
			d.Headers[u.Username()] = u.Password()
		}
	}
	if d.Name == "" {
		switch p := strings.Split(d.URL.Hostname(), "."); len(p) {
		case 1:
			d.Name = cases.Title(language.English).String(p[0])
		default:
			d.Name = cases.Title(language.English).String(p[len(p)-2])
		}
	}

	d.log = NewLogger(d.Name)
	return &d, nil
}

func MustAPI(osVarName string) *API {
	d, err := NewAPI(osVarName)
	if err != nil {
		panic(err)
	}
	return d
}

func (a *API) Endpoint(path string, args ...any) Endpoint[Meta, JSON] {
	return APIEndpoint(a, path, args...)
}

func (a *API) String() string {
	return fmt.Sprintf("%s of %s ", a.Name, a.URL.Host)
}

func (a *API) Header(name, value string, args ...any) *API {
	value = fmt.Sprintf(value, args...)
	if a.Headers == nil {
		a.Headers = make(map[string]string)
	}
	a.Headers[name] = value
	return a
}

// OAuth configures the API with an Authorization function that retrieves
// and caches an access token using the client credentials flow.
//
// It sends a POST request to the given `url` with the provided `params`,
// and appends `client_id` and `client_secret` from API's URL credentials.
//
// On success, the access token is extracted from the response and injected
// into the Authorization header as: "token_type access_token"
//
// The token is cached internally and reused for the duration specified
// in the `expires_in` field of the response. Once expired, a new token
// will be fetched automatically.
func (a *API) OAuth(url string, params ...string) *API {
	a.Authorization = func(req *http.Request) (string, time.Time, error) {
		n := time.Now()
		p := append(params,
			"grant_type=client_credentials",
			"client_id="+a.URL.Username(),
			"client_secret="+a.URL.Password(),
		)
		j, err := NewEndpoint[string, JSON](url).
			Name(a.Name).
			Lock(true).
			Header("Content-Type", "application/x-www-form-urlencoded").
			Header("Authorization", a.URL.BasicAuth()).
			Errors(a.Errors).
			Post(strings.Join(p, "&"))
		if err != nil {
			return "", n, err
		}
		f := time.Duration(j.Number("expires_in")) * time.Second
		return j.Sprintf("%s %s", "token_type", "access_token"), n.Add(f), nil
	}
	return a
}

func (a *API) run(r *http.Request) (*http.Response, error) {
	a.mu.Lock()
	if a.client == nil {
		a.client = &http.Client{
			Timeout: 45 * time.Second,
		}
		if a.Proxy != "" {
			pu, _ := url.Parse(a.Proxy)
			a.client.Transport = &http.Transport{Proxy: http.ProxyURL(pu)}
			a.log.Debugf("with proxy %s", pu)
		}
	}
	a.mu.Unlock()

	tkn, err := a.auth(r)
	if err != nil {
		return nil, err
	}
	if tkn != "" {
		r.Header.Set("Authorization", tkn)
	}
	if InUnitTests() {
		if w, found := Endpoints.handle(r); found {
			return w, nil
		}
	}
	if a.OnRequest != nil && a.OnRequest(r) {
		return a.client.Do(r)
	}
	return a.client.Do(r)
}

func (a *API) auth(r *http.Request) (string, error) {
	var err error
	var tkn string
	var exp time.Time
	key := "rest:tokens:" + a.URL.Host
	if a.Authorization == nil {
		return "", nil
	}
	if b := a.get(r.Context(), key, 1); len(b) != 0 {
		return string(b), nil
	}
	if tkn, exp, err = a.Authorization(r); err != nil {
		return "", err
	}
	if tkn = strings.TrimSpace(tkn); tkn == "" {
		return "", Errorf("token not found")
	}
	a.set(key, []byte(tkn), exp.Sub(time.Now()))
	return tkn, nil
}

func (a *API) get(ctx context.Context, hash string, t time.Duration) []byte {
	if t <= 0 {
		t = a.Cache
	}
	if t <= 0 {
		return nil
	}
	var s string

	if Get(ctx, hash, &s) <= 0 {
		return nil
	}
	return []byte(s)
}

func (a *API) set(hash string, b []byte, t time.Duration) int {
	if t <= 0 {
		t = a.Cache
	}
	if t <= 0 {
		return -1
	}
	return Set(Context(), hash, string(b), t)
}
