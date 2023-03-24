package util

import (
	"encoding/json"
	"fmt"

	log "github.com/alphabill-org/alphabill/internal/logger"
)

func WriteDebugJsonLog(l log.Logger, m string, arg interface{}) {
	if l.GetLevel() >= log.DEBUG {
		j, _ := json.MarshalIndent(arg, "", "\t")
		l.Debug("%s\n%s", m, string(j))
	}
}

func WriteTraceJsonLog(l log.Logger, m string, arg interface{}) {
	if l.GetLevel() >= log.TRACE {
		j, _ := json.MarshalIndent(arg, "", "\t")
		l.Trace("%s\n%s", m, string(j))
	}
}

func EncodeToJsonHelper(arg interface{}) string {
	j, err := json.MarshalIndent(arg, "", "\t")
	if err != nil {
		fmt.Errorf("json encode error: %w", err)
	}
	return string(j)
}
