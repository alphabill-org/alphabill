/*
 * GUARDTIME CONFIDENTIAL
 *
 * Copyright 2008-2020 Guardtime, Inc.
 * All Rights Reserved.
 *
 * All information contained herein is, and remains, the property
 * of Guardtime, Inc. and its suppliers, if any.
 * The intellectual and technical concepts contained herein are
 * proprietary to Guardtime, Inc. and its suppliers and may be
 * covered by U.S. and foreign patents and patents in process,
 * and/or are protected by trade secret or copyright law.
 * Dissemination of this information or reproduction of this material
 * is strictly forbidden unless prior written permission is obtained
 * from Guardtime, Inc.
 * "Guardtime" and "KSI" are trademarks or registered trademarks of
 * Guardtime, Inc., and no license to trademarks is granted; Guardtime
 * reserves and retains all trademark rights.
 */

package logger

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/time"
)

type logLine struct {
	Level                string
	Time                 time.Time
	Message              string
	ContextRound         uint64
	ContextComponentName string
	Caller               string
	GoID                 uint64
}

const (
	ContextRoundKey      = "contextRound"
	ContextComponentName = "contextComponentName"
)

type GlobalLoggerTestSuite struct {
	suite.Suite
	buffer *Buffer
}

func TestGlobalLoggerTestSuite(t *testing.T) {
	suite.Run(t, new(GlobalLoggerTestSuite))
}

func (s *GlobalLoggerTestSuite) SetupTest() {
	initializeGlobalFactory()
}

func (s *GlobalLoggerTestSuite) TestDefaultsWontPanic() {
	assert.NotPanics(s.T(), func() {
		log := CreateForPackage()
		log.Debug("my message")
		log2 := Create("logger2")
		log2.Debug("my message")
	})
}

func (s *GlobalLoggerTestSuite) TestPackageLogger() {
	msg := "my log message"
	s.setupLogging(DEBUG)
	log := CreateForPackage()
	log.Debug(msg)
	s.assertLastLogMessage(msg)

	sameLog1 := Create("internal/logger")
	sameLog1.Debug(msg)
	s.assertLastLogMessage(msg)

	sameLog2 := Create("internal.logger")
	sameLog2.Debug(msg)
	s.assertLastLogMessage(msg)

	sameLog3 := Create("internal_logger")
	sameLog3.Debug(msg)
	s.assertLastLogMessage(msg)

	log.ChangeLevel(INFO)
	s.assertVisibility(log, false, DEBUG)
	s.assertVisibility(sameLog1, false, DEBUG)
	s.assertVisibility(sameLog2, false, DEBUG)
	s.assertVisibility(sameLog3, false, DEBUG)
}

func (s *GlobalLoggerTestSuite) TestSimpleLogger() {
	s.setupLogging(INFO)
	log := Create("logger1")
	s.assertVisibility(log, false, TRACE)
	s.assertVisibility(log, false, DEBUG)
	s.assertVisibility(log, true, INFO)
	s.assertVisibility(log, true, WARNING)
	s.assertVisibility(log, true, ERROR)
}

func (s *GlobalLoggerTestSuite) TestSettingDefaultLevel() {
	logConf := s.setupLogging(INFO)
	log := Create("logger1")
	s.assertVisibility(log, false, DEBUG)
	s.assertVisibility(log, true, INFO)

	// Change global log level should reflect messages
	logConf.DefaultLevel = WARNING
	UpdateGlobalConfig(logConf)
	s.assertVisibility(log, false, INFO)
	s.assertVisibility(log, true, WARNING)
}

func (s *GlobalLoggerTestSuite) TestSettingLevelByLoggerName() {
	logConf := s.setupLoggingWithLoggers(DEBUG, map[string]LogLevel{"logger1": INFO})
	log := Create("logger1")
	s.assertVisibility(log, false, DEBUG)
	s.assertVisibility(log, true, INFO)

	logConf.DefaultLevel = INFO
	logConf.PackageLevels = map[string]LogLevel{"logger1": WARNING}
	UpdateGlobalConfig(logConf)
	s.assertVisibility(log, false, INFO)
	s.assertVisibility(log, true, WARNING)

	// Update without log level should result default for all loggers
	logConf.DefaultLevel = ERROR
	logConf.PackageLevels = nil
	UpdateGlobalConfig(logConf)
	s.assertVisibility(log, false, WARNING)
	s.assertVisibility(log, true, ERROR)
}

func (s *GlobalLoggerTestSuite) TestChangeLevelOnLogger() {
	s.setupLogging(DEBUG)
	log := Create("logger1")
	s.assertVisibility(log, false, TRACE)
	s.assertVisibility(log, true, DEBUG)
	log.ChangeLevel(INFO)
	s.assertVisibility(log, false, TRACE)
	s.assertVisibility(log, false, DEBUG)
	s.assertVisibility(log, true, INFO)
	log.ChangeLevel(TRACE)
	s.assertVisibility(log, true, TRACE)
	s.assertVisibility(log, true, DEBUG)
}

func (s *GlobalLoggerTestSuite) TestContextClearAll() {
	s.setupLogging(DEBUG)
	log := Create("logger1")

	compName := "controller"
	round := uint64(4)
	SetContext(ContextComponentName, compName)
	SetContext(ContextRoundKey, round)
	log.Debug("Message 1")
	s.assertLastLogLine("Message 1", compName, round)

	ClearAllContext()
	log.Debug("Message 2")
	s.assertLastLogLine("Message 2", "", 0)
}

func (s *GlobalLoggerTestSuite) TestContextClearKey() {
	s.setupLogging(DEBUG)
	log := Create("logger1")

	compName := "controller"
	round := uint64(4)
	SetContext(ContextComponentName, compName)
	SetContext(ContextRoundKey, round)
	log.Debug("Message 1")
	s.assertLastLogLine("Message 1", compName, round)

	ClearContext(ContextComponentName)
	log.Debug("Message 2")
	s.assertLastLogLine("Message 2", "", round)

	// Delete twice works too
	ClearContext(ContextComponentName)
	log.Debug("Message 2")
	s.assertLastLogLine("Message 2", "", round)
}

func (s *GlobalLoggerTestSuite) TestConsoleFormat() {
	s.buffer = new(Buffer)
	var (
		conf = GlobalConfig{
			DefaultLevel:  DEBUG,
			Writer:        s.buffer,
			ConsoleFormat: true, // Use console format - non-json
		}
		logLine = &logLine{}
	)
	UpdateGlobalConfig(conf)
	log := Create("logger1")

	log.Debug("message 1")
	line := s.readLogString()
	require.Error(s.T(), json.Unmarshal([]byte(line), logLine), "Should not be JSON")
	assert.Regexp(s.T(), ".*message 1.*", line)

	// Try to switch to JSON
	UpdateGlobalConfig(GlobalConfig{
		DefaultLevel:  DEBUG,
		ConsoleFormat: false,
	})
	s.buffer.Reset()

	log.Debug("message 2")
	line = s.readLogString()
	require.Error(s.T(), json.Unmarshal([]byte(line), logLine), "Should still be console format")
	assert.Regexp(s.T(), ".*message 2.*", line)
}

func (s *GlobalLoggerTestSuite) TestCaller() {
	s.setupLogging(DEBUG) // Sets up logger with "ShowCaller = true"
	log := Create("logger1")

	log.Debug("my message")
	line := s.readLogLine()
	// Won't test exact line number, so it won't break when adding lines
	assert.Regexp(s.T(), ".*global_test.go:.*", line.Caller)

	UpdateGlobalConfig(GlobalConfig{
		DefaultLevel: DEBUG,
		ShowCaller:   false,
	})
	s.buffer.Reset()

	log.Debug("my message")
	line = s.readLogLine()
	assert.Regexp(s.T(), ".*global_test.go:.*", line.Caller, "Caller should still be present. It's not allowed to change caller multiple times")
}

func (s *GlobalLoggerTestSuite) TestGoroutineId() {
	logConf := s.setupLogging(DEBUG)
	log := Create("logger1")

	log.Debug("my message")
	line := s.readLogLine()
	assert.Empty(s.T(), line.GoID)

	logConf.DefaultLevel = WARNING
	logConf.ShowGoroutineID = true
	UpdateGlobalConfig(logConf)
	log.Warning("my message 1")
	line = s.readLogLine()
	assert.NotEmpty(s.T(), line.GoID)

	log.Debug("my message 2")
	assert.Empty(s.T(), s.readLogString())
}

func (s *GlobalLoggerTestSuite) TestConcurrentOperation() {
	s.setupLogging(ERROR)
	testCount := 100
	msgCount := 100
	loggerCount := 5

	test.MustRunInTime(s.T(), time.Second, func() {
		for x := 0; x < testCount; x++ {
			s.buffer.Reset()
			wg := sync.WaitGroup{}
			wg.Add(msgCount*loggerCount + loggerCount)

			for i := 0; i < loggerCount; i++ {
				loggerName := "logger" + strconv.Itoa(i)
				go func() {
					log := Create(loggerName)
					for j := 0; j < msgCount; j++ {
						log.Debug("%s Not visible msg %d", loggerName, j)
						log.Error("%s Visible msg %d", loggerName, j)
						log.ChangeLevel(INFO)
						wg.Done()
					}
				}()
				consoleFormat := i%2 == 0
				showCaller := i%2 == 0
				go func() {
					UpdateGlobalConfig(GlobalConfig{
						DefaultLevel:  INFO,
						PackageLevels: map[string]LogLevel{loggerName: WARNING}, // No effect on log output, but to exercise concurrency
						ConsoleFormat: consoleFormat,
						ShowCaller:    showCaller,
					})
					wg.Done()
				}()

				go func() {
					SetContext("ContextKey", rand.Int())
				}()

				go func() {
					ClearContext("ContextKey")
				}()

				go func() {
					ClearAllContext()
				}()
			}

			wg.Wait()

			allLog := s.buffer.String()
			actualCount := strings.Count(allLog, "Visible msg")
			assert.Equal(s.T(), msgCount*loggerCount, actualCount, "all messages", allLog)
			assert.Empty(s.T(), strings.Count(allLog, "Not visible msg "))
		}
	})
}

func (s *GlobalLoggerTestSuite) setupLogging(defaultLevel LogLevel) GlobalConfig {
	return s.setupLoggingWithLoggers(defaultLevel, nil)
}

func (s *GlobalLoggerTestSuite) setupLoggingWithLoggers(defaultLevel LogLevel, levels map[string]LogLevel) GlobalConfig {
	s.buffer = new(Buffer)
	conf := defaultConfiguration()
	conf.DefaultLevel = defaultLevel
	conf.Writer = s.buffer
	conf.PackageLevels = levels
	conf.ShowCaller = true
	UpdateGlobalConfig(conf)
	return conf
}

func (s *GlobalLoggerTestSuite) assertVisibility(log Logger, expectedVisible bool, level LogLevel) {
	msgSent := "some log message"
	switch level {
	case TRACE:
		log.Trace(msgSent)
	case DEBUG:
		log.Debug(msgSent)
	case INFO:
		log.Info(msgSent)
	case WARNING:
		log.Warning(msgSent)
	case ERROR:
		log.Error(msgSent)
	default:
		s.T().Errorf("Unknown log level  %d", level)
	}

	if expectedVisible {
		s.assertLastLogMessage(msgSent)
	} else {
		require.Empty(s.T(), s.buffer.String())
	}

	msgSent = "some log message with %s"
	param := "param"
	switch level {
	case TRACE:
		log.Trace(msgSent, param)
	case DEBUG:
		log.Debug(msgSent, param)
	case INFO:
		log.Info(msgSent, param)
	case WARNING:
		log.Warning(msgSent, param)
	case ERROR:
		log.Error(msgSent, param)
	default:
		s.T().Errorf("Unknown log level  %d", level)
	}

	if expectedVisible {
		s.assertLastLogMessage(fmt.Sprintf(msgSent, param))
	} else {
		require.Empty(s.T(), s.buffer.String())
	}

}

func (s *GlobalLoggerTestSuite) assertLastLogLine(expectedMessage, expectedCompName string, expectedRound uint64) {
	line := s.readLogLine()
	assert.Equal(s.T(), expectedMessage, line.Message)
	assert.Equal(s.T(), expectedRound, line.ContextRound)
	assert.Equal(s.T(), expectedCompName, line.ContextComponentName)
}

func (s *GlobalLoggerTestSuite) assertLastLogMessage(expectedMessage string) {
	line := s.readLogLine()
	assert.Equal(s.T(), expectedMessage, line.Message)
}

func (s *GlobalLoggerTestSuite) readLogString() string {
	logBuffer := s.buffer.String()
	s.buffer.Reset()
	return logBuffer
}
func (s *GlobalLoggerTestSuite) readLogLine() *logLine {
	line := &logLine{}
	logBuffer := s.buffer.String()
	if err := json.Unmarshal([]byte(logBuffer), line); err != nil {
		require.NoError(s.T(), err, "Could not marshal json, was '%s'", logBuffer)
	}
	s.buffer.Reset()
	return line
}
