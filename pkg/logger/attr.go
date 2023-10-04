package logger

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/libp2p/go-libp2p/core/peer"
)

/*
Log attribute key values. Generally shouldn't be used directly, use
appropriate "attribute constructor function" instead.

Only define names here if they are common for multiple modules, module
specific names should be defined in the module.
*/
const (
	NodeIDKey = "node_id"
	ModuleKey = "module"
	GoIDKey   = "go_id"
	ErrorKey  = "err"
	RoundKey  = "round"
	UnitIDKey = "unit_id"
)

/*
NodeID adds "AB node ID" field.

This function should be used with logger.With() method to create sub-logger
for the node (rather than adding NodeID call to individual logging calls).
*/
func NodeID(id peer.ID) slog.Attr {
	return slog.Any(NodeIDKey, id)
}

/*
Error adds error to the log

	if err:= f(); err != nil {
		log.Error("calling f", logger.Error(err))
	}
*/
func Error(err error) slog.Attr {
	return slog.Any(ErrorKey, err)
}

/*
Round records current round number.
*/
func Round(round uint64) slog.Attr {
	return slog.Uint64(RoundKey, round)
}

/*
UnitID is used to log ID of the primary unit (bill, token, token type,...)
associated to the logging call.
*/
func UnitID(id []byte) slog.Attr {
	return slog.String(UnitIDKey, fmt.Sprintf("%X", id))
}

/*
composeAttrFmt combines attribute formatters into single func.
If input contains nil values those are discarded.
*/
func composeAttrFmt(f ...func(groups []string, a slog.Attr) slog.Attr) func(groups []string, a slog.Attr) slog.Attr {
	f = slices.DeleteFunc(f, func(f func(groups []string, a slog.Attr) slog.Attr) bool { return f == nil })
	switch len(f) {
	case 0:
		return nil
	case 1:
		return f[0]
	case 2:
		f0, f1 := f[0], f[1]
		return func(groups []string, a slog.Attr) slog.Attr {
			return f1(groups, f0(groups, a))
		}
	case 3:
		f0, f1, f2 := f[0], f[1], f[2]
		return func(groups []string, a slog.Attr) slog.Attr {
			return f2(groups, f1(groups, f0(groups, a)))
		}
	default:
		return composeAttrFmt(composeAttrFmt(f[:3]...), composeAttrFmt(f[3:]...))
	}
}

func formatTimeAttr(format string) func(groups []string, a slog.Attr) slog.Attr {
	switch format {
	case "":
		// whatever handler does by default...
		return nil
	case "none":
		return func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		}
	default:
		return func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				if t := a.Value.Time(); !t.IsZero() {
					a.Value = slog.StringValue(t.Format(format))
				}
			}
			return a
		}
	}
}

func formatPeerIDAttr(format string) func(groups []string, a slog.Attr) slog.Attr {
	switch format {
	case "none":
		return func(groups []string, a slog.Attr) slog.Attr {
			if a.Value.Kind() == slog.KindAny {
				if _, ok := a.Value.Any().(peer.ID); ok {
					return slog.Attr{}
				}
			}
			return a
		}
	case "short":
		return func(groups []string, a slog.Attr) slog.Attr {
			if a.Value.Kind() == slog.KindAny {
				if id, ok := a.Value.Any().(peer.ID); ok {
					pid := id.String()
					if len(pid) > 10 {
						pid = fmt.Sprintf("%s*%s", pid[:2], pid[len(pid)-6:])
					}
					a.Value = slog.StringValue(pid)
				}
			}
			return a
		}
	default: // whatever handler does by default, ie long format
		return nil
	}
}

/*
formatAttrECS is a "poor man's ECS handler" ie it formats some well known
attributes according to the ECS spec.
*/
func formatAttrECS(groups []string, a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.MessageKey:
		return slog.String("message", a.Value.String())
	case slog.SourceKey:
		if src, ok := a.Value.Any().(*slog.Source); ok {
			trimSource(src)
			return slog.Group(
				"log",
				slog.Group(
					"origin",
					slog.String("function", src.Function),
					slog.Group("file", slog.String("name", src.File), slog.Int("line", src.Line)),
				),
			)
		}
	case NodeIDKey:
		return slog.Group("service", slog.Group("node", slog.Any("name", a.Value)))
	case ErrorKey:
		return slog.Group("error", slog.Any("message", a.Value.Any()))
	}
	return a
}

/*
trimSource shortens the "function" name field in "src" by trimming the
package name from it.
*/
func trimSource(src *slog.Source) {
	// function name by default includes "full path package name" ie
	// github.com/alphabill-org/alphabill/cli/alphabill/cmd.newBaseCmd.func1
	// so first get last part of the path (filename)...
	_, src.Function = filepath.Split(src.Function)
	// ...and then get rid of package name in front of func name
	if s := strings.SplitAfterN(src.Function, ".", 2); len(s) == 2 {
		src.Function = s[1]
	}
}
