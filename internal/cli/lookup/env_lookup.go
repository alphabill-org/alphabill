package lookup

import (
	"fmt"
	"os"
	"strings"
)

type (
	EnvironmentVariableLookuper string
)

const (
	AlphaBillEnvironmentVariableLookuper = EnvironmentVariableLookuper("AB")
)

func (evl EnvironmentVariableLookuper) LookupVariable(componentName string, key string) (string, bool) {
	envKey := strings.ToUpper(fmt.Sprintf("%s_%s_%s", evl, componentName, key))
	return os.LookupEnv(envKey)
}
