package util

import (
	"encoding/json"

	log "github.com/alphabill-org/alphabill/pkg/logger"
)

func WriteDebugJsonLog(l log.Logger, m string, arg interface{}) {
	if l.GetLevel() >= log.DEBUG {
		j, _ := json.MarshalIndent(arg, "", "\t")
		l.Debug("%s\n%s", m, string(j))
	}
}
