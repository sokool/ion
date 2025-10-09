package ion

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
)

type URL struct{ *url.URL }

// EnvURL retrieves the value of an environment variable and parses it as a URL.
// Returns a URL object or an error if the variable is not set or the URL is invalid.
func EnvURL(varName string) (*URL, error) {
	u, ok := os.LookupEnv(varName)
	if !ok || u == "" {
		return nil, Errorf("url").New("%s ", varName).Wrap(ErrEnvNotFound)
	}
	return NewURL(u)
}

func NewURL(s string, args ...any) (*URL, error) {
	if len(args) > 0 {
		s = fmt.Sprintf(s, args...)
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, Errorf("url: invalid format")
	}
	return &URL{u}, nil
}

func ParseURL(s string, requiredParts ...string) (*URL, error) {
	n, err := NewURL(s)
	if err != nil {
		return nil, err
	}
	if p := n.HasEmpty(requiredParts...); len(p) > 0 {
		return nil, Errorf("url: missing %s parts", p)
	}
	return n, nil
}

func MustURL(s string, args ...any) *URL {
	u, err := NewURL(s, args...)
	if err != nil {
		panic(err)
	}
	return u
}

func (u *URL) Username() string {
	return u.URL.User.Username()
}

func (u *URL) Password() string {
	p, _ := u.URL.User.Password()
	return p
}

func (u *URL) Format(s string) string {
	return strings.NewReplacer(
		"scheme", u.Scheme,
		"host", u.Hostname(),
		"port", u.Port(),
		"user", u.Username(),
		"password", u.Password(),
		"path", u.Path,
		"query", u.RawQuery,
	).Replace(s)
}

func (u *URL) Query(name string) string {
	return u.URL.Query().Get(name)
}

func (u *URL) SetQuery(name, value string) *URL {
	q := u.URL.Query()
	q.Add(name, value)
	u.URL.RawQuery = q.Encode()
	return u
}

func (u *URL) HasEmpty(parts ...string) []string {
	var o []string
	for i := range parts {
		if v := u.Format(parts[i]); v == parts[i] || v == "" {
			o = append(o, parts[i])
		}
	}
	return o
}

func (u *URL) MarshalText() ([]byte, error) {
	return []byte(u.URL.String()), nil
}

// BasicAuth returns the base64 encoded username and password.
func (u *URL) BasicAuth() string {
	s := fmt.Sprintf("%s:%s", u.Username(), u.Password())
	return fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(s)))
}

var ErrEnvNotFound = Errorf("environment variable not found")
