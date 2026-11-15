package ion

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrSQL      = Errorf("sql")
	ErrSQLQuery = ErrSQL.New("query")

	RDBMS                = rdbms{sqlVar: [2]string{"${", "}"}}
	sqlTransactionCtxKey = &struct{}{}
)

type (
	SQLDB       = sql.DB
	SQLTX       = sql.Tx
	sqlExecutor interface {
		ExecContext(Context, string, ...any) (sql.Result, error)
		QueryContext(Context, string, ...any) (*sql.Rows, error)
	}
)

// SQL represents a SQL statement whose result rows are mapped to T.
//
// SQL operations use the default connection unless the first line contains a
// `connection=<name>` directive, for example:
//
//	q := ion.SQL[User](`/* ion:connection=legacy */
//	SELECT id, name FROM users`)
//
// The directive is read from the first line, which remains part of the
// statement and must be valid SQL (typically a comment). A named connection is
// resolved from the corresponding <NAME>_URL environment variable. If it
// cannot be resolved, the operation returns an error instead of falling back
// to the default connection.
type SQL[T any] string

func (s SQL[T]) Write(c Context, tt ...T) error {
	if InUnitTests() {
		return nil
	}
	executor, err := s.connection(c)
	if err != nil {
		return err
	}
	dialect, err := RDBMS.dialect(executor)
	if err != nil {
		return err
	}
	for i := range tt {
		qry, args, err := s.query(tt[i], dialect)
		if err != nil {
			return err
		}
		if _, err = executor.ExecContext(c, qry, args...); err != nil {
			return err
		}
	}
	return nil
}

func (s SQL[T]) Read[P any](ctx Context, params P) ([]T, error) {
	if InUnitTests() {
		return []T{}, nil
	}
	executor, err := s.connection(ctx)
	if err != nil {
		return nil, err
	}
	var tt []T
	err = s.read(ctx, executor, params, func(t T) error { tt = append(tt, t); return nil })
	return tt, err
}

func (s SQL[T]) String() string {
	return string(s)
}

func (s SQL[T]) Stream[P any](ctx Context, params P) <-chan T {
	ch := make(chan T)
	if InUnitTests() {
		close(ch)
		return ch
	}
	if ctx == nil {
		panic("ion.SQL.Stream: nil context")
	}
	to := func(value T) error {
		select {
		case ch <- value:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	go func() {
		executor, err := s.connection(ctx)
		if err == nil {
			err = s.read(ctx, executor, params, to)
		}
		if err != nil {
			log_.Errorf("%s", err)
		}
		close(ch)
	}()
	return ch
}

func (s SQL[T]) Each[P any](ctx Context, params P) Iterator[T, error] {
	if InUnitTests() {
		return func(func(T, error) bool) {}
	}
	return func(fn func(T, error) bool) {
		tt, err := s.Read(ctx, params)
		if err != nil {
			var t T
			fn(t, err)
		}
		for i := range tt {
			if !fn(tt[i], nil) {
				return
			}
		}
	}
}

func (s SQL[T]) read[P any](c Context, e sqlExecutor, params P, to func(T) error) error {
	if c == nil {
		return ErrSQL.New("context is nil")
	}
	if e == nil {
		return ErrSQL.New("query executor is nil")
	}
	dialect, err := RDBMS.dialect(e)
	if err != nil {
		return err
	}
	n := time.Now()
	query, pms, err := s.query(params, dialect)
	if err != nil {
		return err
	}
	rows, err := e.QueryContext(c, query, pms...)
	if err != nil {
		return ErrSQL.Wrap(err)
	}
	defer rows.Close()
	var i int
	for rows.Next() {
		var scn scanner[T]
		if err := rows.Scan(&scn); err != nil {
			return ErrSQL.Wrap(err)
		}
		if err := to(scn.T); err != nil {
			return ErrSQL.Wrap(err)
		}
		i++
	}
	if err := rows.Err(); err != nil {
		return ErrSQL.Wrap(err)
	}
	m := time.Since(n).String()
	var t T
	x := NewReflect(t).Name()
	log_.Trace(4).Debugf(x+": found %d in time of %s %v", i, m, params)
	Metrics.Percentile("sql_read_in_seconds{name=%q}", time.Since(n).Seconds(), x)
	return nil
}

func (s SQL[T]) connection(ctx Context) (sqlExecutor, error) {
	if ctx == nil {
		return nil, ErrSQL.New("context is nil")
	}
	if tx, ok := ctx.Value(sqlTransactionCtxKey).(*SQLTX); ok {
		return s.transaction(ctx, tx)
	}

	line, _, _ := strings.Cut(string(s), "\n")
	const directive = "connection="
	i := strings.Index(line, directive)
	var (
		db  *SQLDB
		err error
	)
	if i < 0 {
		db, err = RDBMS.Connection(ctx)
	} else {
		name := line[i+len(directive):]
		if end := strings.IndexAny(name, " \t\r*/;"); end >= 0 {
			name = name[:end]
		}
		if name == "" {
			return nil, ErrSQLQuery.New("connection name is empty")
		}
		db, err = RDBMS.Connection(ctx, name)
	}
	if err != nil {
		return nil, err
	}
	return db, nil
}

func (s SQL[T]) transaction(ctx Context, tx *SQLTX) (sqlExecutor, error) {
	if ctx == nil {
		return nil, ErrSQL.New("context is nil")
	}
	if tx == nil {
		return nil, ErrSQL.New("transaction is nil")
	}
	value, ok := RDBMS.transactions.Load(tx)
	if !ok {
		return nil, ErrSQL.New("transaction was not created by RDBMS.Transaction")
	}
	db := value.(*SQLDB)

	line, _, _ := strings.Cut(string(s), "\n")
	const directive = "connection="
	if i := strings.Index(line, directive); i >= 0 {
		name := line[i+len(directive):]
		if end := strings.IndexAny(name, " \t\r*/;"); end >= 0 {
			name = name[:end]
		}
		if name == "" {
			return nil, ErrSQLQuery.New("connection name is empty")
		}
		selectedDB, err := RDBMS.Connection(ctx, name)
		if err != nil {
			return nil, err
		}
		if selectedDB != db {
			return nil, ErrSQL.New("query connection does not match transaction")
		}
	}
	return tx, nil
}

// query processes SQL query template by replacing variables in format described by
// sqlPrefix and sqlPostfix with dialect-specific placeholders and collecting corresponding
// values from params. Variable names can include dots and array indexes to access nested
// fields. MySQL uses a placeholder for every occurrence; PostgreSQL-style connections
// use numbered placeholders and reuse them for repeated variables.
//
// Parameters:
//   - params: Any object containing values for query parameters
//
// Returns:
//   - string: Processed SQL query with placeholders for the selected dialect
//   - []any: Slice of parameter values corresponding to placeholders
//   - error: Error if query processing fails
func (s SQL[T]) query[P any](params P, dialect string) (string, []any, error) {
	in := string(s)
	if in == "" {
		return in, nil, nil
	}

	vars := RDBMS.variables()
	if vars[0] == "" {
		return "", nil, ErrSQLQuery.New("variable prefix cannot be empty, use RDBMS.SQLWrapVars")
	}

	seg := `[A-Za-z_][A-Za-z0-9_]*(?:$begin:math:display$[0-9]+$end:math:display$)*`
	name := `(?P<name>` + seg + `(?:\.` + seg + `)*)`
	epo := ""
	ep := regexp.QuoteMeta(vars[0])
	if vars[1] != "" {
		epo = regexp.QuoteMeta(vars[1])
	}
	re, err := regexp.Compile(ep + name + epo)
	if err != nil {
		return "", nil, ErrSQLQuery.New("variable pre,post fix compilation %w", err)
	}

	var (
		sb     strings.Builder
		as     []any
		r      = NewReflect(params)
		values = make(map[string]any)
		mv     = make(map[string]int) // name -> 1-based index
		p0     = 0
	)

	for _, m := range re.FindAllStringSubmatchIndex(in, -1) {
		// m: full match start/end, then capture group start/end (name is group 1).
		a, e := m[0], m[1]
		ns, ne := m[2], m[3] // captured name

		if ns < 0 || ne < 0 {
			continue
		}
		name := in[ns:ne]

		value, ok := values[name]
		if !ok {
			value, err = r.Get(name)
			if err != nil {
				return "", nil, ErrSQLQuery.New("%q %w", name, err)
			}
			values[name] = value
		}

		sb.WriteString(in[p0:a])
		if dialect == "mysql" {
			sb.WriteByte('?')
			as = append(as, valuer{value})
		} else {
			idx, ok := mv[name]
			if !ok {
				as = append(as, valuer{value})
				idx = len(as)
				mv[name] = idx
			}
			sb.WriteByte('$')
			sb.WriteString(strconv.Itoa(idx))
		}
		p0 = e
	}
	sb.WriteString(in[p0:])

	return sb.String(), as, nil
}

type rdbms struct {
	mu           sync.RWMutex
	connections  sync.Map
	dialects     sync.Map
	transactions sync.Map
	sqlVar       [2]string
}

// SQLWrapVars sets the prefix and postfix used for SQL variable interpolation.
// Examples:
//
//	RDBMS.SQLWrapVars("{", "}")
//	// Produces variables like: {id}, {name}
//
//	RDBMS.SQLWrapVars(":", "")
//	// Produces variables like: :id, :name
func (s *rdbms) SQLWrapVars(prefix, postfix string) {
	s.mu.Lock()
	s.sqlVar = [2]string{prefix, postfix}
	s.mu.Unlock()
}

func (s *rdbms) variables() [2]string {
	s.mu.RLock()
	vars := s.sqlVar
	s.mu.RUnlock()
	return vars
}

func (s *rdbms) dialect(e sqlExecutor) (string, error) {
	var db *SQLDB
	switch connection := e.(type) {
	case *SQLDB:
		db = connection
	case *SQLTX:
		value, ok := s.transactions.Load(connection)
		if !ok {
			return "", ErrSQL.New("transaction was not created by RDBMS.Transaction")
		}
		db = value.(*SQLDB)
	default:
		return "", ErrSQL.New("unsupported SQL executor %T", e)
	}

	dialect, ok := s.dialects.Load(db)
	if !ok {
		return "", ErrSQL.New("dialect not found for SQL connection")
	}
	return dialect.(string), nil
}

// Connection establishes a new SQL connection or returns an existing one.
// It retrieves connection settings from environment variables, based on `name`.
// The function applies connection pooling configurations if provided in the URL.
// Thread-safe and avoids duplicate connections using sync.Map.
func (s *rdbms) Connection(ctx Context, name ...string) (*SQLDB, error) {
	if len(name) == 0 {
		name = append(name, "postgres", "mysql")
	}
	if ctx == nil {
		return nil, ErrSQL.New("context is nil")
	}
	var url *URL
	var err error
	var env string
	for i := range name {
		env = fmt.Sprintf("%s_URL", strings.ToUpper(name[i]))
		if db, ok := s.connections.Load(env); ok {
			return db.(*SQLDB), nil
		}
		switch url, err = EnvURL(env); {
		case ErrEnvNotFound.In(err):
			err = ErrSQL.Wrap(ErrEnvNotFound)
			continue
		case err != nil:
			return nil, ErrSQL.Wrap(err)
		}

		break
	}
	if err != nil {
		return nil, err
	}
	u := url.String()
	switch url.Scheme {
	case "mysql":
		u = url.Format("user:password@tcp(host:port)path?query")
	}
	db, err := sql.Open(url.Scheme, u)
	if err != nil {
		return nil, ErrSQL.Wrap(err)
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, ErrSQL.Wrap(err)
	}

	if n, err := Cast[string, int](url.Query("max_open")); err == nil {
		db.SetMaxOpenConns(n)
	}
	if n, err := Cast[string, int](url.Query("max_idle")); err == nil {
		db.SetMaxIdleConns(n)
	}
	if d, err := Cast[string, time.Duration](url.Query("max_lifetime")); err == nil {
		db.SetConnMaxLifetime(d)
	}
	// double-check pattern (avoid duplicate connections)
	s.dialects.Store(db, url.Scheme)
	actual, loaded := s.connections.LoadOrStore(env, db)
	if loaded {
		s.dialects.Delete(db)
		db.Close() // discard new one, keep old
		return actual.(*SQLDB), nil
	}
	log_.Infof("%s initialized", url.Scheme)
	return db, nil
}

// Transaction runs fn within a transaction on the default or optional named
// SQL connection. SQL operations in fn use the transaction context automatically.
func (s *rdbms) Transaction(ctx Context, fn func(Context) error, connection ...string) error {
	if len(connection) > 1 || (len(connection) == 1 && connection[0] == "") {
		return ErrSQL.New("transaction accepts at most one non-empty connection name")
	}
	if ctx == nil {
		return ErrSQL.New("context is nil")
	}
	if fn == nil {
		return ErrSQL.New("transaction callback is nil")
	}
	if InUnitTests() {
		return nil
	}
	db, err := s.Connection(ctx, connection...)
	if err != nil {
		return err
	}
	if _, ok := s.dialects.Load(db); !ok {
		return ErrSQL.New("dialect not found for SQL transaction")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ErrSQL.New("tx: begin %w", err)
	}
	s.transactions.Store(tx, db)
	defer s.transactions.Delete(tx)

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // rethrow panic
		}
	}()

	txCtx := context.WithValue(ctx, sqlTransactionCtxKey, tx)
	if err := fn(txCtx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

type scanner[T any] struct{ T T }

func (f *scanner[T]) Scan(src any) error {
	switch v := src.(type) {
	case []byte:
		err := json.Unmarshal(v, &f.T)
		return err
	case string:
		err := json.Unmarshal([]byte(v), &f.T)
		return err
	case nil:
		//*f = Foo{} // reset
		return nil
	default:
		return fmt.Errorf("foo: unsupported scan type %T", v)
	}
}

// Val is a generic wrapper that implements driver.Valuer for any T.
type valuer struct {
	V any
}

// Value converts the underlying T into driver.Value.
func (w valuer) Value() (driver.Value, error) {
	switch v := w.V.(type) {
	case nil:
		return nil, nil
	case int:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case uint:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint64:
		// clamp: values beyond int64 range will overflow
		if v > uint64(math.MaxInt64) {
			panic("accept: uint64 overflows int64")
		}
		return int64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case bool:
		return v, nil
	case string:
		return v, nil
	case []byte:
		return v, nil
	case time.Time:
		return v, nil
	case driver.Valuer:
		return v.Value()
	case interface{ Value() (any, error) }:
		return v.Value()
	case fmt.Stringer:
		return v.String(), nil
	default:
		return fmt.Sprint(v), nil
	}
}
