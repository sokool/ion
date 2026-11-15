package ion

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestSQLQueryDialects(t *testing.T) {
	query := SQL[struct{}](`SELECT ${ID}, ${OTHER}, ${ID}`)
	params := struct {
		ID    int
		OTHER int
		Tags  []string
	}{ID: 7, OTHER: 8, Tags: []string{"tag"}}

	tests := []struct {
		name       string
		dialect    string
		query      string
		wantParams []int64
	}{
		{
			name:       "postgres reuses numbered placeholders",
			dialect:    "postgres",
			query:      "SELECT $1, $2, $1",
			wantParams: []int64{7, 8},
		},
		{
			name:       "mysql repeats positional arguments",
			dialect:    "mysql",
			query:      "SELECT ?, ?, ?",
			wantParams: []int64{7, 8, 7},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotQuery, gotParams, err := query.query(params, test.dialect)
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			if gotQuery != test.query {
				t.Errorf("query = %q, want %q", gotQuery, test.query)
			}
			if len(gotParams) != len(test.wantParams) {
				t.Fatalf("got %d params, want %d", len(gotParams), len(test.wantParams))
			}
			for i, param := range gotParams {
				value, ok := param.(valuer)
				if !ok {
					t.Fatalf("param %d has type %T, want valuer", i, param)
				}
				gotValue, err := value.Value()
				if err != nil {
					t.Fatalf("param %d value: %v", i, err)
				}
				if gotValue != test.wantParams[i] {
					t.Errorf("param %d = %v, want %d", i, gotValue, test.wantParams[i])
				}
			}
		})
	}
}

func TestRDBMSSQLWrapVars(t *testing.T) {
	vars := RDBMS.variables()
	t.Cleanup(func() { RDBMS.SQLWrapVars(vars[0], vars[1]) })
	RDBMS.SQLWrapVars("{", "}")

	query := SQL[int]("SELECT {ID}")
	got, _, err := query.query(struct{ ID int }{ID: 7}, "postgres")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if got != "SELECT $1" {
		t.Errorf("query = %q, want %q", got, "SELECT $1")
	}
}

func TestSQLMethodsAcceptNonComparableParams(t *testing.T) {
	type params struct {
		ID   int
		Tags []string
	}

	query := SQL[int]("SELECT ${ID}")
	input := params{ID: 42, Tags: []string{"tag"}}
	ctx := context.Background()

	if got, err := query.Read(ctx, input); err != nil || len(got) != 0 {
		t.Fatalf("All: %v", err)
	}
	called := false
	query.Each(ctx, input)(func(_ int, err error) bool {
		called = true
		if err != nil {
			t.Errorf("Each: %v", err)
		}
		return true
	})
	if called {
		t.Error("Each yielded a result in a unit test")
	}
	for range query.Stream(ctx, input) {
		t.Error("Stream yielded a result in a unit test")
	}
}

func TestSQLMethodsAreNoOpsInUnitTests(t *testing.T) {
	query := SQL[int]("SELECT ${ID}")
	params := struct{ ID int }{ID: 42}

	if got, err := query.Read(nil, params); err != nil || len(got) != 0 {
		t.Fatalf("Read = %v, %v; want empty results and no error", got, err)
	}
	if err := query.Write(nil, 42); err != nil {
		t.Fatalf("Write: %v", err)
	}
	called := false
	if err := RDBMS.Transaction(context.Background(), func(Context) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("Transaction: %v", err)
	}
	if called {
		t.Fatal("transaction callback ran in a unit test")
	}
	if _, err := RDBMS.Connection(nil); err == nil {
		t.Fatal("RDBMS.Connection succeeded with a nil context")
	}
	if err := RDBMS.Transaction(nil, func(Context) error { return nil }); err == nil {
		t.Fatal("RDBMS.Transaction succeeded with a nil context")
	}

	for range query.Stream(nil, params) {
		t.Error("Stream yielded a result in a unit test")
	}
}

func TestSQLConnectionDirective(t *testing.T) {
	db := new(SQLDB)
	cacheSQLConnection(t, "ion_test_legacy", db, "mysql")
	cacheSQLConnection(t, "postgres", new(SQLDB), "postgres")

	query := SQL[int]("/* ion:connection=ion_test_legacy */\nSELECT 1")
	type foo struct {
		X *int
	}
	for range query.Stream(context.Background(), foo{}) {
		t.Error("Stream yielded a result in a unit test")
	}
	gotDB, err := query.connection(context.Background())
	if err != nil {
		t.Fatalf("connection: %v", err)
	}
	dialect, err := RDBMS.dialect(gotDB)
	if err != nil {
		t.Fatalf("dialect: %v", err)
	}
	if gotDB != db {
		t.Errorf("connection = %p, want %p", gotDB, db)
	}
	if dialect != "mysql" {
		t.Errorf("dialect = %q, want mysql", dialect)
	}

	bar()
}

func TestSQLConnectionUsesDefaultWithoutDirective(t *testing.T) {
	db := new(SQLDB)
	cacheSQLConnection(t, "postgres", db, "postgres")

	query := SQL[int]("SELECT 1")
	gotDB, err := query.connection(context.Background())
	if err != nil {
		t.Fatalf("connection: %v", err)
	}
	dialect, err := RDBMS.dialect(gotDB)
	if err != nil {
		t.Fatalf("dialect: %v", err)
	}
	if gotDB != db {
		t.Errorf("connection = %p, want %p", gotDB, db)
	}
	if dialect != "postgres" {
		t.Errorf("dialect = %q, want postgres", dialect)
	}
}

func TestSQLConnectionDirectiveDoesNotFallback(t *testing.T) {
	cacheSQLConnection(t, "postgres", new(SQLDB), "postgres")

	name := "ion_test_missing_connection"
	env := strings.ToUpper(name) + "_URL"
	oldDB, hadDB := RDBMS.connections.LoadAndDelete(env)
	t.Cleanup(func() {
		if hadDB {
			RDBMS.connections.Store(env, oldDB)
		}
	})
	t.Setenv(env, "")

	query := SQL[int]("/* ion:connection=" + name + " */\nSELECT 1")
	db, err := query.connection(context.Background())
	if err == nil {
		t.Fatal("connection succeeded for an unconfigured alias")
	}
	if db != nil {
		t.Errorf("connection = %p, want nil", db)
	}
}

func TestSQLTransactionUsesTransactionConnection(t *testing.T) {
	db := new(SQLDB)
	cacheSQLConnection(t, "ion_test_transaction", db, "mysql")

	tx := new(SQLTX)
	RDBMS.transactions.Store(tx, db)
	t.Cleanup(func() { RDBMS.transactions.Delete(tx) })

	query := SQL[int]("SELECT ${ID}")
	ctx := context.WithValue(context.Background(), sqlTransactionCtxKey, tx)
	executor, err := query.connection(ctx)
	if err != nil {
		t.Fatalf("transaction: %v", err)
	}
	dialect, err := RDBMS.dialect(executor)
	if err != nil {
		t.Fatalf("dialect: %v", err)
	}
	gotTx, ok := executor.(*SQLTX)
	if !ok || gotTx != tx {
		t.Errorf("executor = %T, want transaction %p", executor, tx)
	}
	if dialect != "mysql" {
		t.Errorf("dialect = %q, want mysql", dialect)
	}
}

func TestSQLTransactionRejectsMismatchedDirective(t *testing.T) {
	txDB := new(SQLDB)
	cacheSQLConnection(t, "ion_test_tx", txDB, "postgres")
	cacheSQLConnection(t, "ion_test_query", new(SQLDB), "mysql")

	tx := new(SQLTX)
	RDBMS.transactions.Store(tx, txDB)
	t.Cleanup(func() { RDBMS.transactions.Delete(tx) })

	query := SQL[int]("/* ion:connection=ion_test_query */\nSELECT ${ID}")
	ctx := context.WithValue(context.Background(), sqlTransactionCtxKey, tx)
	if _, err := query.connection(ctx); err == nil {
		t.Fatal("transaction succeeded with a mismatched connection directive")
	}
}

func TestSQLQueryKeepsConnectionDirective(t *testing.T) {
	query := SQL[int]("/* ion:connection=legacy */\nSELECT 1")

	got, _, err := query.query(struct{}{}, "mysql")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if got != string(query) {
		t.Errorf("query = %q, want unchanged statement %q", got, query)
	}
}

func TestSQLStringReturnsQuery(t *testing.T) {
	query := SQL[int]("SELECT 1")
	if got := query.String(); got != string(query) {
		t.Errorf("String() = %q, want %q", got, query)
	}
}

func TestSQLScannerDecodesJSON(t *testing.T) {
	type row struct {
		ID int `json:"id"`
	}

	for _, test := range []struct {
		name string
		src  any
	}{
		{name: "string", src: `{"id":42}`},
		{name: "bytes", src: []byte(`{"id":42}`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got scanner[row]
			if err := got.Scan(test.src); err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if got.T.ID != 42 {
				t.Errorf("ID = %d, want 42", got.T.ID)
			}
		})
	}
}

func cacheSQLConnection(t *testing.T, name string, db *SQLDB, dialect string) {
	t.Helper()

	env := strings.ToUpper(name) + "_URL"
	oldDB, hadDB := RDBMS.connections.Load(env)
	oldDialect, hadDialect := RDBMS.dialects.Load(db)
	RDBMS.connections.Store(env, db)
	RDBMS.dialects.Store(db, dialect)

	t.Cleanup(func() {
		if hadDB {
			RDBMS.connections.Store(env, oldDB)
		} else {
			RDBMS.connections.Delete(env)
		}
		if hadDialect {
			RDBMS.dialects.Store(db, oldDialect)
		} else {
			RDBMS.dialects.Delete(db)
		}
	})
}

func bar() {
	ctx := context.Background()
	for u, err := range users.Each(ctx, Query{Age: 42}) {
		if err != nil {
			panic(err)
		}
		fmt.Println(u)
	}
}

type User struct {
	ID   string
	Name string
}

type Query struct {
	Age int
}

var users = SQL[User]("select * from users")
