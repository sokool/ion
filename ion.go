package ion

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"os/signal"
	"strings"

	"github.com/google/uuid"
)

var (
	env     string
	Tasks   *Jobs
	Metrics *metrics
	Cache   Store = &memory{}

	ctx     context.Context
	log_    = NewLogger(os.Getenv("APP_NAME"))
	cancel  func()
	website string
)

func init() {
	env = os.Getenv("ION_ENVIRONMENT")
	ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt)
	Tasks = NewJobs(ctx)
	Metrics = NewMetrics()

	website, _ = os.LookupEnv("WEBSITE")

	fmt.Println()
}

// Context with signal handler for graceful shutdowns. When process is closed then
// context sends done signal
func Context() context.Context {
	return ctx
}

// Exit terminates the program with an exit code depending on the presence of errors in args.
func Exit(msg string, args ...any) {
	cancel()
	log_.Trace(1).Printf(msg, args...)
	code := 0
	for i := range args {
		if _, ok := args[i].(error); ok {
			code = 1
			break
		}
	}
	os.Exit(code)
}

// InProduction checks if the current environment is suitable for production-level operations.
// The environment is determined by the 'WORKFLOW_API_ENVIRONMENT' or 'ENVIRONMENT' OS variable.
//
// Parameters:
//   - strict (optional): A variadic boolean parameter. If provided and true, the function
//     demands the environment be strictly "production". Otherwise, it considers both
//     "production" and "demo" as acceptable environments.
//
// Returns:
//   - bool: true if the environment is "production" or, if not strict, "demo"; false otherwise.
func InProduction(strict ...bool) bool {
	if len(strict) > 0 && strict[0] {
		return env == "production"
	}
	return env == "production" || env == "demo"
}

// InUnitTests checks if the running binary has a suffix of ".test".
func InUnitTests() bool {
	return strings.HasSuffix(os.Args[0], ".test")
}

// Website returns a fully-qualified URL string based on the current environment.
// It takes a URL path (e.g., "/login") and optional fmt.Sprintf-style args for formatting.
//
// Example:
//
//	Website("/user/%d", 42) => "https://test.com/user/42" (in production)
//
// It supports three environments:
//   - "production" → https://test.com
//   - "demo"       → https://demo.test.com
//   - anything else (including dev) → http://localhost
func Website(path string, args ...any) string {
	w := website + path
	return fmt.Sprintf(w, args...)
}

// UUID generates a Universally Unique Identifier string.
// If one or more strings are provided as arguments, it creates a UUID
// based on the SHA-1 hash of the concatenated strings using the DNS namespace.
// If no arguments are provided, it generates a random UUID.
//
// Parameters:
//
// s: Optional variadic string arguments. If provided, these strings are concatenated and
// used to generate a SHA-1 based UUID.
//
// Returns:
//
// A string representing the generated UUID.
//
// Example usage:
//
// randomUUID := UUID() // Generates a random UUID
// namedUUID := UUID("example", "test") // Generates a UUID based on the provided strings
func UUID(s ...string) string {
	if len(s) > 0 {
		var k []byte
		for i := range s {
			k = append(k, s[i]...)
		}
		return uuid.NewSHA1(uuid.NameSpaceDNS, k).String()
	}
	return uuid.New().String()
}

func Printf(s any) {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		panic(fmt.Sprintf("cannot marshal: %v", err))
	}
	fmt.Println(string(b))
}

type (
	Iterator[K, V any] = iter.Seq2[K, V]
)
