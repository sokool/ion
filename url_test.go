package ion

import (
	"testing"
)

func TestNewURL(t *testing.T) {
	n := "https://tom%40gmail.com:myCooLPswd@api.test.com/oauth/token"
	u, err := NewURL(n)
	if err != nil {
		t.Fatal(err)
	}
	if u.Username() != "tom@gmail.com" {
		t.Fatal("expected 'tom@gmail.com', got", u.Username())
	}
	if u.Password() != "myCooLPswd" {
		t.Fatal("expected 'myCooLPswd', got", u.Password())
	}
	if s := u.BasicAuth(); s != "Basic dG9tQGdtYWlsLmNvbTpteUNvb0xQc3dk" {
		t.Fatal("expected 'Basic dG9tQGdtYWlsLmNvbTpteUNvb0xQc3dk', got", s)
	}
	if s, err := u.MarshalText(); err != nil || string(s) != n {
		t.Fatal("expected", n, "got", string(s), err)
	}
}

func TestURL_Format(t *testing.T) {
	u, err := NewURL("https://portal.equinix.com:8081/oauth/token")
	if err != nil {
		t.Fatal(err)
	}
	if s := u.Format("host:port"); s != "portal.equinix.com:8081" {
		t.Fatal("expected 'https://portal.equinix.com/v1path', got", s)
	}
}
