package logger

import (
	"runtime"
	"strings"
)

type PackageNameResolver struct {
	BasePackage string
	Depth       int
}

func (r *PackageNameResolver) PackageName() string {
	pc, _, _, _ := runtime.Caller(r.depth())
	// For example: github.com/alphabill-org/alphabill/internal/logger.TestPackageName
	pcName := runtime.FuncForPC(pc).Name()
	split1 := strings.SplitN(pcName, r.BasePackage, 2)
	var packageAfterBase string
	if len(split1) < 2 {
		// If it was not inside base package
		packageAfterBase = split1[0]
	} else {
		split2 := strings.SplitN(split1[1], ".", 2)
		packageAfterBase = split2[0]
	}
	return strings.Trim(packageAfterBase, "/")
}

func (r *PackageNameResolver) depth() int {
	// 2 because it's used from inside logging code. We want the caller of that.
	if r.Depth == 0 {
		return 2
	}
	return r.Depth
}
