package util

import (
	"encoding/json"
	"fmt"

	log "github.com/alphabill-org/alphabill/pkg/logger"
)

// EncodeToJsonHelper encodes to argument to json and returns it as string,
// on encode error return error message as string
func EncodeToJsonHelper(arg interface{}) string {
	j, err := json.MarshalIndent(arg, "", "\t")
	if err != nil {
		fmt.Errorf("json encode error: %w", err)
	}
	return string(j)
}

func WriteDebugJsonLog(l log.Logger, m string, arg interface{}) {
	if l.GetLevel() >= log.DEBUG {
		l.Debug("%s\n%s", m, EncodeToJsonHelper(arg))
	}
}

func WriteTraceJsonLog(l log.Logger, m string, arg interface{}) {
	if l.GetLevel() >= log.TRACE {
		l.Trace("%s\n%s", m, EncodeToJsonHelper(arg))
	}
}
