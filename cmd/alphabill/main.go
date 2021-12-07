package main

import (
	"context"

	"gitdc.ee.guardtime.com/alphabill/alphabill/cmd/alphabill/cmd"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async"
)

func main() {
	ctx, _ := async.WithWaitGroup(context.Background())
	cmd.New().Execute(ctx)
}
