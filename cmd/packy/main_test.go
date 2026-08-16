package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestSecondInterruptForcesExitAfterFirstCancelsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	interrupts := make(chan os.Signal, 2)
	done := make(chan struct{})
	forced := make(chan struct{}, 1)
	go forceOnSecondInterrupt(ctx, interrupts, done, func() { forced <- struct{}{} })

	interrupts <- os.Interrupt
	cancel()
	select {
	case <-forced:
		t.Fatal("first interrupt forced exit")
	case <-time.After(25 * time.Millisecond):
	}
	interrupts <- os.Interrupt
	select {
	case <-forced:
	case <-time.After(time.Second):
		t.Fatal("second interrupt did not force exit")
	}
}

func TestInterruptWatcherStopsWhenCommandCompletes(t *testing.T) {
	ctx := context.Background()
	interrupts := make(chan os.Signal, 2)
	done := make(chan struct{})
	forced := make(chan struct{}, 1)
	go forceOnSecondInterrupt(ctx, interrupts, done, func() { forced <- struct{}{} })

	close(done)
	interrupts <- os.Interrupt
	interrupts <- os.Interrupt
	select {
	case <-forced:
		t.Fatal("completed command retained an active interrupt watcher")
	case <-time.After(25 * time.Millisecond):
	}
}
