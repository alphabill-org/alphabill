package logger

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/alphabill-org/alphabill-go-base/types"
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
	DataKey   = "data"

	traceID = "TraceId" // OTEL data model
	spanID  = "SpanId"  // OTEL data model
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
Data adds additional data field to the message.

slog.GroupValue shouldn't be used as the data - in the ECS formatter all
groups will end up under the same key possibly causing problems with index!

Use of anonymous types is discouraged too.
*/
func Data(d any) slog.Attr {
	return slog.Any(DataKey, d)
}

/*
UnitID is used to log ID of the primary unit (bill, token, token type,...)
associated to the logging call.
*/
func UnitID(id []byte) slog.Attr {
	return slog.String(UnitIDKey, fmt.Sprintf("%X", id))
}

/*
Shard creates shard id/partition id attribute.

Shard specific components (ie shard validator) should create logger which adds this
attribute automatically, ie RootChain should use it when logging message specific to
particular shard.
*/
func Shard(partition types.PartitionID, shard types.ShardID) slog.Attr {
	return slog.Group("shard",
		slog.Uint64("partition", uint64(partition)),
		slog.String("id", shard.String()),
	)
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

func formatDataAttrAsJSON(groups []string, a slog.Attr) slog.Attr {
	if a.Key == DataKey {
		switch a.Value.Kind() {
		case slog.KindAny:
			if b, err := json.Marshal(a.Value.Any()); err == nil {
				a.Value = slog.StringValue(string(b))
			}
		}
	}
	return a
}

/*
formatAttrWallet strips everything except a minimal set of attributes so
that the log output is minimal (hopefully better suitable for end users).
*/
func formatAttrWallet(groups []string, a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.LevelKey, slog.MessageKey, ErrorKey:
		return a
	default:
		return slog.Attr{}
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
	case DataKey:
		// to keep Elastic happy we nest the actual value under it's type name, kind of namespacing it.
		// as ie `data:"string value"` and `data: 42` would cause type conflict in Elastic index.
		// different struct types might also have a field with the same name but different type!
		return slog.Group(DataKey, slog.Any(dataName(a.Value), a.Value))
	case traceID:
		return slog.Group("trace", slog.String("id", a.Value.String()))
	case spanID:
		return slog.Group("span", slog.String("id", a.Value.String()))
	}
	return a
}

/*
dataName returns name of the data type of "v", suitable to act as a "namespace" for
the value in ECS format. There is basically no restrictions for key names in JSON
but this func attempts to do some sanitizing in order to make querying the resulting
JSON a bit easier.
*/
func dataName(v slog.Value) string {
	switch v.Kind() {
	case slog.KindAny, slog.KindLogValuer:
		a := v.Any()
		// for anonymous type reflect.TypeOf(a).String() returns type def, ie
		// struct { Str string; Int int }
		// which is valid but not nice JSON key. For now we do not worry about
		// that (just do not use anon types for data)!
		rt := reflect.TypeOf(a)
		// strip leading "*" of pointer types and replace "." with "_"
		return strings.ReplaceAll(strings.TrimLeft(rt.String(), "*"), ".", "_")
	default:
		return v.Kind().String()
	}
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
