package logger

type Logger interface {
	Trace(format string, args ...interface{})
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warning(format string, args ...interface{})
	Error(format string, args ...interface{})
	// Changes logger level to the newLevel
	ChangeLevel(newLevel LogLevel)
}

type LogLevel uint

const (
	NONE LogLevel = iota
	ERROR
	WARNING
	INFO
	DEBUG
	TRACE
)

func LevelFromString(s string) LogLevel {
	switch s {
	case "NONE":
		return NONE
	case "ERROR":
		return ERROR
	case "WARNING":
		return WARNING
	case "INFO":
		return INFO
	case "DEBUG":
		return DEBUG
	case "TRACE":
		return TRACE

	default:
		return DEBUG
	}
}
