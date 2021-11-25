package viper

import (
	"strings"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"

	jww "github.com/spf13/jwalterweatherman"
)

type (
	viperLogWriter struct {
		writeFunc func(format string, args ...interface{})
	}
)

func (w *viperLogWriter) Write(p []byte) (n int, err error) {
	if w == nil || w.writeFunc == nil {
		return 0, errors.Wrap(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	w.writeFunc("%s", strings.TrimSpace(string(p)))
	return len(p), nil
}

func init() {
	log := logger.Create("github.com/spf13/viper")
	jww.SetLogThreshold(jww.LevelTrace)
	jww.SetFlags(0)
	jww.SetPrefix("github.com/spf13/viper")
	jww.SetLogOutput(&viperLogWriter{writeFunc: log.Debug})
}
