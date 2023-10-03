package debug

import (
	"runtime/debug"
	"strings"
)

func ReadBuildInfo() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var vcsData []string
	for _, s := range info.Settings {
		if strings.HasPrefix(s.Key, "vcs.") {
			vcsData = append(vcsData, s.Key+"="+s.Value)
		}
	}
	return strings.Join(vcsData, " ")
}
