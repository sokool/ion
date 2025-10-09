package ion

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"embed"
	"encoding/json"
	"fmt"
	"hash/fnv"
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
)

type (
	SQLDB             = sql.DB
	SQLTX             = sql.Tx
	Collection[T any] interface {
		Append(T) error
		Get(int) (T, bool)
	}
)

type SQL[T any] string

func (s SQL[T]) Read(ctx context.Context, to Collection[T]) error {
	return s.scan(ctx, to, to.Append)
}

func (s SQL[T]) Write(c context.Context, tt ...T) error {
	db, err := SQLConnection(ctx)
	if err != nil {
		return err
	}
	cxt := c
	if cxt == nil {
		cxt, _ = context.WithTimeout(ctx, time.Second*5)
	}
	for _, t := range tt {
		qry, args, err := s.query(t)
		if err != nil {
			return err
		}
		if InUnitTests() {
			continue
		}
		if _, err = db.ExecContext(cxt, qry, args...); err != nil {
			return err
		}
	}
	return nil
}

func (s SQL[T]) All(ctx context.Context, params any) ([]T, error) {
	var o []T
	return o, s.scan(ctx, params, func(t T) error { o = append(o, t); return nil })
}

func (s SQL[T]) One(ctx context.Context, params any) (T, error) {
	var t T
	var fn = func(n T) error { t = n; return nil }
	if err := s.scan(ctx, params, fn); err != nil {
		return t, err
	}
	return t, nil
}

func (s SQL[T]) String() string {
	return ""
}

func (s SQL[T]) TX(c *SQLTX) SQL[T] {
	return s
}

func (s SQL[T]) Stream(params any) <-chan T {
	ch := make(chan T)
	fn := func(t T) error { ch <- t; return nil }
	go func() {
		if err := s.scan(nil, params, fn); err != nil {
			log_.Errorf("%s", err)
		}
		close(ch)
	}()
	return ch
}

func (s SQL[T]) Each(ctx context.Context, params any) Iterator[T, error] {
	return func(fn func(T, error) bool) {
		tt, err := s.All(ctx, params)
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

func (s SQL[T]) Foo(n string) SQL[T] {
	fmt.Println(s.hash())
	return s
}

func (s SQL[T]) hash() uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

func (s SQL[T]) scan(c context.Context, params any, to func(T) error) error {
	n := time.Now()
	if c == nil {
		c, _ = context.WithTimeout(ctx, time.Second*5)
	}
	qry, pms, err := s.query(params)
	if err != nil {
		return err
	}
	if InUnitTests() {
		return nil
	}
	db, err := SQLConnection(ctx)
	if err != nil {
		return err
	}
	rows, err := db.QueryContext(c, qry, pms...)
	if err != nil {
		return ErrSQL.Wrap(err)
	}
	defer rows.Close()
	var i int
	for rows.Next() {
		var scn scanner[T]
		if err = rows.Scan(&scn); err != nil {
			return ErrSQL.Wrap(err)
		}
		if err = to(scn.T); err != nil {
			return ErrSQL.Wrap(err)
		}
		i++
	}
	if err = rows.Err(); err != nil {
		return ErrSQL.Wrap(err)
	}
	m := time.Since(n).String()
	//x := NewReflect[T]().Name()
	x := "TODO"
	log_.Trace(4).Debugf(x+": found %d in time of %s, based on %s", i, m, s)
	Metrics.Percentile("sql_read_in_seconds{name=%q}", time.Since(n).Seconds(), x)
	return nil
}

// query processes SQL query template by replacing variables in format described by
// sqlPrefix and sqlPostfix with $N placeholders and collecting corresponding values from
// the params object. Variable names can include dots and array indexes to access nested
// fields. Returns the processed query string, slice of parameter values, and any
// error that occurred during processing.
//
// Parameters:
//   - params: Any object containing values for query parameters
//
// Returns:
//   - string: Processed SQL query with $N placeholders
//   - []any: Slice of parameter values corresponding to placeholders
//   - error: Error if query processing fails
func (s SQL[T]) query(params any) (string, []any, error) {
	in := string(s)
	if in == "" {
		return in, nil, nil
	}

	if sqlVar[0] == "" {
		return "", nil, ErrSQLQuery.New("variable prefix cannot be empty, use app.SQLPrefix")
	}

	seg := `[A-Za-z_][A-Za-z0-9_]*(?:$begin:math:display$[0-9]+$end:math:display$)*`
	name := `(?P<name>` + seg + `(?:\.` + seg + `)*)`
	epo := ""
	ep := regexp.QuoteMeta(sqlVar[0])
	if sqlVar[1] != "" {
		epo = regexp.QuoteMeta(sqlVar[1])
	}
	re, err := regexp.Compile(ep + name + epo)
	if err != nil {
		return "", nil, ErrSQLQuery.New("variable pre,post fix compilation %w", err)
	}

	var (
		sb strings.Builder
		as []any
		r  = NewReflect(params)
		i  = 0
		mv = make(map[string]int) // name -> 1-based index
		p0 = 0
	)

	for _, m := range re.FindAllStringSubmatchIndex(in, -1) {
		// m: full match start/end, then capture group start/end (name is group 1).
		a, e := m[0], m[1]
		ns, ne := m[2], m[3] // captured name

		if ns < 0 || ne < 0 {
			continue
		}
		name := in[ns:ne]

		idx, ok := mv[name]
		if !ok {
			v, err := r.Get(name)
			if err != nil {
				return "", nil, ErrSQLQuery.New("%q %w", name, err)
			}
			as = append(as, valuer{v})
			i++
			idx = i
			mv[name] = idx
		}

		sb.WriteString(in[p0:a])
		sb.WriteString("$")
		sb.WriteString(strconv.Itoa(idx))
		p0 = e
	}
	sb.WriteString(in[p0:])

	return sb.String(), as, nil
}

func SQLMigrate(schema, table string, files embed.FS) error {
	//return RDBMS.AcquireFunc(ctx, func(c *pgxpool.Conn) error {
	//	cfg := dbump.Config{
	//		Migrator: dbump_pgx.NewMigrator(c.Conn(), dbump_pgx.Config{Schema: schema, Table: table}),
	//		Loader:   dbump.NewFileSysLoader(files, "migrations/"),
	//		Mode:     dbump.ModeApplyAll,
	//	}
	//	return dbump.Run(ctx, cfg)
	//})
	return nil
}

var sqlConnections sync.Map

// SQLConnection establishes a new SQL connection or returns an existing one.
// It retrieves connection settings from environment variables, based on `name`.
// The function applies connection pooling configurations if provided in the URL.
// Thread-safe and avoids duplicate connections using sync.Map.
func SQLConnection(ctx context.Context, name ...string) (*SQLDB, error) {
	if InUnitTests() {
		return nil, nil
	}

	if len(name) == 0 {
		name = append(name, "postgres", "mysql")
	}
	var url *URL
	var err error
	var env string
	for i := range name {
		env = fmt.Sprintf("%s_URL", strings.ToUpper(name[i]))
		if db, ok := sqlConnections.Load(env); ok {
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
	db, err := sql.Open(url.Scheme, url.String())
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
	actual, loaded := sqlConnections.LoadOrStore(env, db)
	if loaded {
		db.Close() // discard new one, keep old
		return actual.(*SQLDB), nil
	}
	log_.Infof("%s initialized", url.Scheme)
	return db, nil
}

func SQLTransaction(ctx context.Context, fn func(*SQLTX) error) error {
	if InUnitTests() {
		return nil
	}
	db, err := SQLConnection(ctx)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ErrSQL.New("tx: begin %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // rethrow panic
		}
	}()

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

var sqlVar = [2]string{"${", "}"}

// SQLWrapVars sets the prefix and postfix used for SQL variable interpolation.
// This function defines how variables should be wrapped when constructing SQL
// statements.
// Examples:
//
//	SQLWrapVars("{", "}")
//	// Produces variables like: {id}, {name}
//
//	SQLWrapVars(":", "")
//	// Produces variables like: :id, :name
//
// Use this to customize the variable markers for different SQL dialects.
func SQLWrapVars(prefix, postfix string) {
	sqlVar[0], sqlVar[1] = prefix, postfix
}

type scanner[T any] struct{ T T }

func (f *scanner[T]) Scan(src any) error {
	switch v := src.(type) {
	case []byte:
		err := json.Unmarshal(v, &f.T)
		return err
	case string:
		err := json.Unmarshal([]byte(v), f.T)
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
