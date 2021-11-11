package logger

import (
	"testing"
)

func BenchmarkLoggerWithoutContext(b *testing.B) {
	b.ReportAllocs()

	log := setupLogger()
	for i := 0; i < b.N; i++ {
		log.Info("Logging some stuff with args %d", i)
	}
}

func BenchmarkLoggerWithContextChange(b *testing.B) {
	b.ReportAllocs()

	log := setupLogger()
	contextAfter := 100
	for i := 0; i < b.N; i++ {
		if i%contextAfter == 0 {
			SetContext("roundIndex", uint64(i))
		}
		log.Info("Logging some stuff with args %d", i)
	}
}

func setupLogger() Logger {
	buffer := new(Buffer)
	UpdateGlobalConfig(GlobalConfig{
		DefaultLevel: DEBUG,
		Writer:       buffer,
		ShowCaller:   false,
	})

	log := CreateForPackage()
	return log
}
