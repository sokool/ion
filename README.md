# ion

A Go toolkit for common backend tasks: HTTP clients, JSON, SQL, caching, background jobs,
logging, metrics, pub/sub, and LLM integrations. All features live in the `ion` package.

## Installation

Requires Go 1.25.1 or newer.

```sh
go get github.com/sokool/ion
```

## Quick start

Make an HTTP request and read the JSON response:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sokool/ion"
)

func main() {
	ctx, cancel := context.WithTimeout(ion.Context(), 10*time.Second)
	defer cancel()

	api, err := ion.APIFromURL("https://jsonplaceholder.typicode.com")
	if err != nil {
		log.Fatal(err)
	}

	post, err := api.Endpoint("/posts/%d", 1).Context(ctx).Get()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(post.Text("title"))
}
```

`API.Endpoint` uses `ion.Meta` request bodies and `ion.JSON` responses. For typed request
and response values, use `ion.NewEndpoint[Request, Response](url)`.

Endpoints support headers, query parameters, custom methods, response caching, and custom
error handlers. Set a context deadline explicitly; the HTTP client has no default timeout.

## Features

| Area | Main APIs |
| --- | --- |
| HTTP clients | `APIFromURL`, `NewAPI`, `NewEndpoint`, `JSONEndpoint` |
| JSON | `JSON`, `Meta`, path selection, decoding, mutation, merging, iteration |
| SQL | `SQL[T]`, `SQLConnection`, `SQLTransaction`, `SQLWrapVars` |
| Storage | `Store`, `UseStore`, `Get`, `Set`, global `Cache` |
| Background jobs | `NewJobs`, global `Tasks`, `RunAsync` |
| Logging | `NewLogger`, backed by [`sokool/log`](https://github.com/sokool/log) |
| Metrics | `NewMetrics`, global `Metrics`, Prometheus-compatible HTTP handler |
| Messaging | `Topic[T]`, `NewTopic`, `PubSub`, `UsePubSub` |
| Coordination | `Locker`, `UseLocker`, `Limiter`, `UseLimiter` |
| LLMs | `LLM`, `Prompt`, `LLMChat`, `NewTool`, `NewToolMD` |
| Other helpers | `URL`, `Text`, `Errorf`, `UUID`, `Reflect`, `Cast` |

## JSON

`JSON` wraps JSON bytes and provides path-based access without defining a struct first.
Paths support dot notation and GJSON queries.

The following snippets assume the `ion` import and run inside a function.

```go
data := ion.JSON(`{"user":{"name":"Ada"},"tags":["go","backend"]}`)

fmt.Println(data.Text("user.name")) // Ada
fmt.Println(data.Select("tags.0"))  // "go"

if err := data.Set("user.active", true); err != nil {
	return err
}

var user struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}
if err := data.Select("user").To(&user); err != nil {
	return err
}
```

Use `ion.Meta` for JSON objects built in Go:

```go
body := ion.Meta{"name": "Ada", "active": true}
fmt.Println(body.JSON().Text("name")) // Ada
```

## Storage and adapters

`ion.Set` JSON-encodes values; `ion.Get` decodes them into a destination pointer.
`Set` returns the byte count or `-1` on error. `Get` returns the byte count, `0` for
a missing key, or `-1` on error.

```go
ctx := ion.Context()
if ion.Set(ctx, "user:42", ion.Meta{"name": "Ada"}) < 0 {
	return fmt.Errorf("could not store user")
}

var user ion.Meta
if n := ion.Get(ctx, "user:42", &user); n > 0 {
	fmt.Println(user["name"])
}
```

The default store is an in-memory map. It **does not expire entries or synchronize
concurrent access**. Register a suitable `Store` with `ion.UseStore` before using caching
in concurrent applications or relying on TTLs.

Other infrastructure can also be replaced through `UsePubSub`, `UseLocker`, and
`UseLimiter`. Configure these global adapters at startup, before starting workers.
The default locker creates a new local mutex per call; it does not coordinate separate
calls or processes. Cross-worker locking requires a custom adapter.

## Background jobs

Jobs accept an empty schedule for a one-off run, a duration such as `"30m"`, a clock time
such as `"15:04"`, or a weekday and time such as `"Monday 15:04"`.

```go
jobs := ion.NewJobs(ion.Context())
defer jobs.Close()

err := jobs.Run("heartbeat", func(ctx context.Context) error {
	ion.NewLogger("heartbeat").Infof("still running")
	return nil
}, "30m")
if err != nil {
	return err
}
jobs.Wait()
```

A scheduled job with no stored execution time runs immediately on its first slot.
Later runs follow the schedule. `Wait` blocks until the jobs context is canceled;
it does not wait for individual job functions to finish.

For concurrent scheduled jobs, configure a concurrency-safe store. Coordination between
workers also requires an appropriate locker adapter.

## Metrics

Record counters, histograms, and summaries, and expose them over HTTP:

```go
ion.Metrics.Count(`requests_total{service=%q}`, 1, "example")
ion.Metrics.Histogram("request_duration_seconds", 0.025)

mux := http.NewServeMux()
mux.Handle("/metrics", ion.Metrics)
```

The handler exports Prometheus-compatible application and process metrics.

## SQL

`SQL[T]` represents a query with typed results. Parameters use `${Field}` markers by
default; ion resolves them from the supplied value and replaces them with `$1`, `$2`,
and so on. Use `SQLWrapVars` to change the marker delimiters.

- Register a `database/sql` driver in your application; ion does not bundle drivers.
- `SQLConnection(ctx)` looks for `POSTGRES_URL`, then `MYSQL_URL`. An explicit name such
  as `SQLConnection(ctx, "analytics")` selects `ANALYTICS_URL`.
- The URL scheme must match the registered SQL driver name.
- Typed reads expect one JSON column per row, decoded into `T`, rather than one column
  per struct field.
- Queries use PostgreSQL-style numbered placeholders; changing marker delimiters does
  not change the generated placeholder syntax.
- SQL reads, writes, and transaction callbacks are skipped in Go test binaries.
- `SQLMigrate` and `SQL[T].TX` are currently placeholders; they do not apply migrations
  or bind a query to a transaction.

## LLM integrations

`LLM` supports ChatGPT-compatible and Gemini APIs. `Prompt` adds templating, while
`LLMChat` maintains conversations. Tools can be defined from typed Go functions or
Markdown descriptions.

Configure `CHATGPT_URL` for ChatGPT-compatible requests, or `GEMINI_URL` for models
whose names start with `gemini`. For example:

```sh
export CHATGPT_URL='https://bearer:YOUR_API_KEY@api.openai.com'
```

```go
llm := ion.LLM{
	Model:       "gpt-4.1-mini",
	Instruction: "Answer briefly and precisely.",
}
answer, err := llm.Read("What is a Go interface?")
if err != nil {
	return err
}
fmt.Println(answer)
```

## Runtime configuration

| Variable | Purpose |
| --- | --- |
| `APP_NAME` | Tag for the package-level logger. |
| `ION_ENVIRONMENT` | `production` or `demo` makes `InProduction()` return true. |
| `WEBSITE` | Base string prepended by `Website(path, args...)`. |
| `POSTGRES_URL`, `MYSQL_URL` | Default SQL connection URLs. |
| `CHATGPT_URL`, `GEMINI_URL` | LLM API connection settings. |

`NewAPI("SERVICE_URL")` reads any application-defined environment variable containing
an API URL. API URLs can include options such as `Cache=5m`, `Name=Service`, and
`Header.Accept=application/json`. Bearer authentication can be specified using URL
credentials in the form `https://bearer:TOKEN@api.example.com`.

Importing ion initializes a shared context that is canceled on `os.Interrupt`, along
with the default logger, job manager, metrics set, and store. It also prints a blank
line to standard output. `ion.Exit` cancels that context, logs a message, and calls
`os.Exit`; use it only at the application boundary.

## Development

```sh
go test ./...
```

See the [package reference](https://pkg.go.dev/github.com/sokool/ion) for exported APIs.
