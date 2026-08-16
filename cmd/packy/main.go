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
	cmd := cli.NewRootCommand(cli.Options{})
	// Keep signal notification active until the command returns. Repeated
	// interrupts therefore preserve the TUI's non-interruptible Apply boundary.
	err := cmd.ExecuteContext(ctx)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
