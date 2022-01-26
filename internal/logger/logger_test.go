package logger

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadGlobalConfigFromFile(t *testing.T) {
	fileLogOutput, err := os.CreateTemp("", "example-log-output.*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fileLogOutput.Name())

	configFileContents := fmt.Sprintf(`
outputPath: %s
consoleFormat: false
showCaller: true
timeLocation: UTC
showGoroutineID: true
defaultLevel: DEBUG
packageLevels:
  internal_txbuffer: WARNING
`, fileLogOutput.Name())

	// Write temporary file and clean up afterwards
	fileConf, err := os.CreateTemp("", "example-logger-conf.*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fileConf.Name())
	if _, err = fileConf.Write([]byte(configFileContents)); err != nil {
		t.Fatal(err)
	}
	if err = fileConf.Close(); err != nil {
		t.Fatal(err)
	}

	globalConf, err := loadGlobalConfigFromFile(fileConf.Name())
	require.NoError(t, err)

	require.NotNil(t, globalConf.Writer)
	require.False(t, globalConf.ConsoleFormat)
	require.True(t, globalConf.ShowCaller)
	require.Equal(t, "UTC", globalConf.TimeLocation)
	require.True(t, globalConf.ShowGoroutineID)
	require.Equal(t, DEBUG, globalConf.DefaultLevel)
	require.Equal(t, WARNING, globalConf.PackageLevels["internal_txbuffer"])
}
