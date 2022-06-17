//go:build !race

package logger

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadGlobalConfigFromFile(t *testing.T) {
	_, fileConf := setupTest(t)

	globalConf, err := loadGlobalConfigFromFile(fileConf)
	require.NoError(t, err)

	require.NotNil(t, globalConf.Writer)
	require.False(t, globalConf.ConsoleFormat)
	require.True(t, globalConf.ShowCaller)
	require.Equal(t, "UTC", globalConf.TimeLocation)
	require.True(t, globalConf.ShowGoroutineID)
	require.Equal(t, DEBUG, globalConf.DefaultLevel)
	require.Equal(t, WARNING, globalConf.PackageLevels["internal_txbuffer"])
}

func TestLoggingToFile(t *testing.T) {
	initializeGlobalFactory()
	logFilePath, confFilePath := setupTest(t)
	err := UpdateGlobalConfigFromFile(confFilePath)
	require.NoError(t, err)

	log := Create("test_logger")
	log.Info("my message")

	logFileContent, err := os.ReadFile(logFilePath)
	require.NoError(t, err)
	require.Contains(t, string(logFileContent), "my message")
}

func setupTest(t *testing.T) (string, string) {
	fileLogOutput, err := os.CreateTemp("", "example-log-output.*.txt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(fileLogOutput.Name())
	})

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
	t.Cleanup(func() {
		_ = os.Remove(fileConf.Name())
	})
	if _, err = fileConf.Write([]byte(configFileContents)); err != nil {
		t.Fatal(err)
	}
	if err = fileConf.Close(); err != nil {
		t.Fatal(err)
	}
	return fileLogOutput.Name(), fileConf.Name()
}
