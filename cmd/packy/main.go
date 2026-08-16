package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/yersonargotev/packy/internal/cli"
)

func main() {
	interrupts := make(chan os.Signal, 2)
	signal.Notify(interrupts, os.Interrupt)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	watcherDone := make(chan struct{})
	go forceOnSecondInterrupt(ctx, interrupts, watcherDone, func() { os.Exit(130) })
	cmd := cli.NewRootCommand(cli.Options{})
	err := cmd.ExecuteContext(ctx)
	close(watcherDone)
	signal.Stop(interrupts)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func forceOnSecondInterrupt(ctx context.Context, interrupts <-chan os.Signal, done <-chan struct{}, force func()) {
	select {
	case <-ctx.Done():
	case <-done:
		return
	}
	select {
	case <-interrupts:
	case <-done:
		return
	}
	select {
	case <-interrupts:
		force()
	case <-done:
	}
}
