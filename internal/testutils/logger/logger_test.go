package logger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/libp2p/go-libp2p/core/peer"
)

func Test_logger_for_tests(t *testing.T) {
	t.Skip("this test is only for visually checking the output")

	t.Run("first", func(t *testing.T) {
		l := New(t)
		l.Error("now thats really bad", logger.Error(fmt.Errorf("what now")))
		l.Warn("going to tell it just once")
		l.Info("so you know")
		l.Debug("lets investigate")
		t.Error("calling t.Error causes the test to fail")
	})

	t.Run("second", func(t *testing.T) {
		l := NewLvl(t, slog.LevelInfo)
		l.Error("now thats really bad", logger.Error(fmt.Errorf("what now")))
		l.Warn("going to tell it just once")
		l.Info("so you know")
		t.Log("this is INFO level logger so Debug call should not show up")
		l.Debug("this shouldn't show up in the log")
		t.Fail()
	})
}

func Test_logger_for_tests_color(t *testing.T) {
	t.Skip("this test is only for visually checking the output")

	t.Run("colors disabled", func(t *testing.T) {
		t.Setenv("AB_TEST_LOG_NO_COLORS", "true")

		l := New(t)
		l.Error("now thats really bad", logger.Error(fmt.Errorf("what now")))
		l.Warn("going to tell it just once")
		l.Info("so you know")
		l.Debug("lets investigate")
		t.Error("calling t.Error causes the test to fail")
	})

	t.Run("colors enabled", func(t *testing.T) {
		t.Setenv("AB_TEST_LOG_NO_COLORS", "false")

		l := New(t)
		l.Error("now thats really bad", logger.Error(fmt.Errorf("what now")))
		l.Warn("going to tell it just once")
		l.Info("so you know")
		l.Debug("lets investigate")
		t.Error("calling t.Error causes the test to fail")
	})
}

func Test_loggers_json_output(t *testing.T) {
	log, err := logger.New(&logger.LogConfiguration{OutputPath: "stdout", Level: "debug", Format: "ecs"})
	if err != nil {
		for ; err != nil; err = errors.Unwrap(err) {
			t.Logf("%T : %v", err, err)
		}
		t.Fatalf("initializing logger: %v", err)
	}

	nodeID, err := peer.Decode("16Uiu2HAm7xi5YRtfsXd4w7UXomcd5T75o44JmrYrAedS7NGszzek")
	if err != nil {
		t.Errorf("failed to decode peer id: %v", err)
	}

	type foo struct {
		V string
	}

	log.LogAttrs(context.Background(),
		slog.LevelInfo,
		"some information",
		logger.Error(fmt.Errorf("additional error message")),
		logger.NodeID(nodeID),
		logger.UnitID([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}),
		logger.Data(&foo{"bar"}),
	)
}
