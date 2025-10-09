package ion_test

import (
	"os"
	"strings"
	"testing"

	"ion"
)

func TestEndpoint(t *testing.T) {
	u := "https://api.restful-api.dev/objects"
	e := ion.JSONEndpoint(u).
		Name("test").
		Header("Accept", "application/json;schema=simple").
		Query("type", "building").
		Query("text", "hudson avenue ca")
	if dcs, err := e.Execute(); err != nil || dcs.IsEmpty() {
		t.Fatal(err)
	}

}

func TestNewDomain(t *testing.T) {
	os.Setenv("EMPTY_URL", "")
	_, err := ion.NewAPI("EMPTY_URL", false)
	if err == nil {
		t.Error("expected error")
	}
}

func TestDomain(t *testing.T) {
	api := &ion.API{}
	out, err := api.Endpoint("/hello").Post(ion.JSON{})
	if err == nil {
		t.Fatal("error expected")
	}
	if out != nil {
		t.Fatal("nil expected")
	}
	if !strings.Contains(err.Error(), "domain url not found") {
		t.Fatal("error does not contain 'domain url not found'")
	}
}
