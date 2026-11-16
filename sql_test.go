package ion_test

import (
	"io"
	"testing"

	. "github.com/sokool/ion"
)

func TestSQLReadReturnsEmptyInUnitTests(t *testing.T) {
	query := SQL[int]("SELECT ${ID}")
	params := struct {
		ID   int
		Tags []string
	}{ID: 42, Tags: []string{"tag"}}

	got, err := query.Read(t.Context(), params)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Read returned %d rows, want none in unit tests", len(got))
	}
}

func TestSQLQueryReaderReturnsEOFInUnitTests(t *testing.T) {
	query := SQL[int]("SELECT ${ID}")
	reader := query.Query(struct{ ID int }{ID: 42})
	defer reader.Close()

	got := make([]int, 2)
	n, err := reader.Read(t.Context(), got)
	if n != 0 || err != io.EOF {
		t.Fatalf("Read = %d, %v; want 0, io.EOF", n, err)
	}
}

func TestSQLWriteIsNoOpInUnitTests(t *testing.T) {
	if err := SQL[int]("INSERT INTO values (${ID})").Write(t.Context(), 42); err != nil {
		t.Fatalf("Write: %v", err)
	}
}

func TestSQLEachReturnsNoRowsInUnitTests(t *testing.T) {
	query := SQL[int]("SELECT ${ID}")
	called := false

	query.Each(t.Context(), struct{ ID int }{ID: 42})(func(_ int, _ error) bool {
		called = true
		return true
	})
	if called {
		t.Fatal("Each yielded a row in a unit test")
	}
}

func TestSQLStreamReturnsNoRowsInUnitTests(t *testing.T) {
	for range SQL[int]("SELECT 1").Stream(t.Context(), struct{}{}) {
		t.Fatal("Stream yielded a row in a unit test")
	}
}

func TestSQLStringReturnsQuery(t *testing.T) {
	query := SQL[int]("SELECT 1")
	if got := query.String(); got != "SELECT 1" {
		t.Errorf("String() = %q, want %q", got, "SELECT 1")
	}
}

func TestRDBMSConnectionRejectsNilContext(t *testing.T) {
	if _, err := RDBMS.Connection(nil); err == nil {
		t.Fatal("Connection succeeded with a nil context")
	}
}

func TestRDBMSTransactionIsNoOpInUnitTests(t *testing.T) {
	called := false
	err := RDBMS.Transaction(t.Context(), func(Context) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}
	if called {
		t.Fatal("transaction callback ran in a unit test")
	}
}
