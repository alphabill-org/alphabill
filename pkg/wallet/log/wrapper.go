package log

import (
	"fmt"

	pkglogger "github.com/alphabill-org/alphabill/pkg/logger"
)

type wrapper struct {
	l pkglogger.Logger
}

func InitLogger(logger pkglogger.Logger) {
	SetLogger(&wrapper{l: logger})
}

func (w *wrapper) Debug(v ...interface{}) {
	if w.l == nil {
		return
	}
	w.l.Debug(fmt.Sprint(v...))
}

func (w *wrapper) Info(v ...interface{}) {
	if w.l == nil {
		return
	}
	w.l.Info(fmt.Sprint(v...))
}

func (w *wrapper) Notice(v ...interface{}) {
	if w.l == nil {
		return
	}
	w.l.Info(fmt.Sprint(v...))
}

func (w *wrapper) Warning(v ...interface{}) {
	if w.l == nil {
		return
	}
	w.l.Warning(fmt.Sprint(v...))
}

func (w *wrapper) Error(v ...interface{}) {
	if w.l == nil {
		return
	}
	w.l.Error(fmt.Sprint(v...))
}
