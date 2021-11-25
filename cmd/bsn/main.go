package main

import (
	"context"
	"os"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
)

var log = logger.CreateForPackage()

func main() {
	ctx, _ := async.WithWaitGroup(context.Background())
	err := runBillShardNode(ctx, &configuration{})
	if err != nil {
		log.Error("Received an error when running BSN: %s", err)
		os.Exit(1)
	}
}
