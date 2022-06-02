package util

import (
	"encoding/json"

	log "gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
)

func WriteDebugJsonLog(l log.Logger, m string, arg interface{}) {
	if l.GetLevel() >= log.DEBUG {
		j, _ := json.MarshalIndent(arg, "", "\t")
		l.Debug("%s\n%s", m, string(j))
	}
}
