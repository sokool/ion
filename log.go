package ion

import (
	"os"

	"github.com/sokool/log"
)

type Logger = log.Logger

func NewLogger(name string, traceDepth ...int) *Logger {
	l := log.New(os.Stdout, log.All).Tag(name)
	if len(traceDepth) > 0 {
		return l.Trace(traceDepth[0])
	}
	return l
}
