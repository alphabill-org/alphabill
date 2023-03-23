package starter

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alphabill-org/alphabill/internal/async"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

type (
	// StartFunc must initialize and then return. I.e. not block.
	// It must listen for the ctx.Done() and release resources when it happens.
	StartFunc func(ctx context.Context)
)

var log = logger.CreateForPackage()

// StartAndWait handles wait groups, cancels and signals on its own
// The context passed as an argument has to have a ContextWaitGroup context value defined,
// otherwise StartAndWait will return with an error.
//
// StartAndWait executes startFunc and waits for context wait group, if context wait group is empty then startFunc does
// not block and returns immediately,
// startFunc can be terminated using SIGTERM signal, context cancel or timeout StartupOption.
//
// componentName - used for logging purposes to identify which component is started and shut down.
// startFunc - function to call when initialization is done. The function must initialize and then return. I.e. not block.
func StartAndWait(ctx context.Context, componentName string, startFunc StartFunc, opts ...StartupOption) error {
	options := aggregateStartupOptions(opts)

	wg, err := async.WaitGroup(ctx)
	if err != nil {
		return err
	}

	ctx, ctxCancel := context.WithCancel(ctx)
	defer ctxCancel()

	log.Info("Creating component %s", componentName)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	wgWaitDone := make(chan bool, 1)
	exitError := make(chan error, 1)
	go func() {
		select {
		case sig := <-sigChan:
			log.Info("Received signal '%s'. Starting graceful shutdown of %s", sig, componentName)
			ctxCancel()
		case <-ctx.Done():
			log.Info("Received context done. Starting graceful shutdown of %s", componentName)
		case <-wgWaitDone:
			exitError <- nil
			return
		}

		shutdownTimer := time.NewTimer(options.gracefulShutdownTimeout)
		defer shutdownTimer.Stop()
		select {
		case <-wgWaitDone:
			exitError <- nil
		case <-shutdownTimer.C:
			exitError <- errors.Wrapf(errors.ErrTimeout,
				"failed to gracefully shutdown component %s in time.",
				componentName)
		}
	}()

	log.Info("Starting component %s", componentName)
	startFunc(ctx)
	log.Info("Component started: %s", componentName)

	go func() {
		wg.Wait()
		wgWaitDone <- true
		log.Info("Finished graceful shutdown of component %s", componentName)
	}()

	return <-exitError
}
