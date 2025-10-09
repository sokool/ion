package ion_test

import (
	"os"
	"testing"

	"ion"
)

func TestNewDomainPanic(t *testing.T) {
	os.Setenv("ABC_URL", "tcp://onet.pl?Cache=24h&MaxRequestsPerSecond=32.2")
	if _, err := ion.NewAPI("ABC_URL"); err != nil {
		t.Fatalf("NewDomain: %s", err)
	}
}
