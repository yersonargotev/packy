package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/yersonargotev/packy/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		stop()
	}()
	cmd := cli.NewRootCommand(cli.Options{})
	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
