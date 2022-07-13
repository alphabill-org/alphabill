package main

import (
	"context"

	"github.com/alphabill-org/alphabill/cli/alphabill/cmd"
	"github.com/alphabill-org/alphabill/internal/async"
)

func main() {
	ctx, _ := async.WithWaitGroup(context.Background())
	cmd.New().Execute(ctx)
}
