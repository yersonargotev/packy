package tui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type blockingLifecycleBackend struct {
	loadStarted       chan struct{}
	initializeStarted chan struct{}
	previewStarted    chan struct{}
}

func (b *blockingLifecycleBackend) Load(ctx context.Context) (Dashboard, error) {
	close(b.loadStarted)
	<-ctx.Done()
	return Dashboard{}, ctx.Err()
}

func (b *blockingLifecycleBackend) Initialize(ctx context.Context, progress func(string)) error {
	close(b.initializeStarted)
	<-ctx.Done()
	progress("late progress")
	return ctx.Err()
}

func (b *blockingLifecycleBackend) Preview(ctx context.Context, _ PreviewRequest) (Preview, error) {
	close(b.previewStarted)
	<-ctx.Done()
	return Preview{}, ctx.Err()
}

func (*blockingLifecycleBackend) Apply(context.Context, ApplyRequest, func(ApplyProgress)) (ApplyResult, error) {
	return ApplyResult{}, nil
}

type cancellationBackend struct {
	applyContext    context.Context
	loadContextErr  error
	loadHadDeadline bool
}

func (b *cancellationBackend) Load(ctx context.Context) (Dashboard, error) {
	b.loadContextErr = ctx.Err()
	_, b.loadHadDeadline = ctx.Deadline()
	return Dashboard{}, ctx.Err()
}

func (*cancellationBackend) Initialize(context.Context, func(string)) error { return nil }

func (*cancellationBackend) Preview(context.Context, PreviewRequest) (Preview, error) {
	return Preview{}, nil
}

func (b *cancellationBackend) Apply(ctx context.Context, _ ApplyRequest, progress func(ApplyProgress)) (ApplyResult, error) {
	b.applyContext = ctx
	progress(ApplyProgress{Phase: "verification"})
	return ApplyResult{Stage: "verification", Verified: true}, ctx.Err()
}

func TestOperationWaitReturnsCancellationWithoutAnEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan tea.Msg)
	cancel()

	message := waitForInitialization(ctx, events)()
	finished, ok := message.(initializationFinished)
	if !ok || !errors.Is(finished.err, context.Canceled) {
		t.Fatalf("wait result = %#v; want initialization cancellation", message)
	}
}

func TestCanceledOperationSenderDoesNotBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan tea.Msg)
	cancel()
	done := make(chan struct{})

	go func() {
		sendOperationEvent(ctx, events, initializationProgress{detail: "late"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled operation sender remained blocked")
	}
}

func TestRunCancellationStopsBlockedLoad(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	input, inputWriter := io.Pipe()
	defer inputWriter.Close()
	backend := &blockingLifecycleBackend{loadStarted: make(chan struct{}), initializeStarted: make(chan struct{}), previewStarted: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, backend, input, &bytes.Buffer{})
	}()
	<-backend.loadStarted
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run cancellation error = %v; want clean request exit", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run remained blocked after load cancellation")
	}
}

func TestInitializationAndPreviewObserveModelCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	backend := &blockingLifecycleBackend{loadStarted: make(chan struct{}), initializeStarted: make(chan struct{}), previewStarted: make(chan struct{})}
	model := newModel(ctx, backend)

	_, initializeCommand := model.startInitialization()
	initializeBatch := initializeCommand().(tea.BatchMsg)
	initializeDone := make(chan tea.Msg, 1)
	go func() { initializeDone <- initializeBatch[0]() }()
	<-backend.initializeStarted
	cancel()
	finished := initializeBatch[1]().(initializationFinished)
	if !errors.Is(finished.err, context.Canceled) {
		t.Fatalf("initialization error = %v; want cancellation", finished.err)
	}
	select {
	case <-initializeDone:
	case <-time.After(time.Second):
		t.Fatal("initialization producer remained blocked after cancellation")
	}

	previewCtx, previewCancel := context.WithCancel(context.Background())
	previewModel := newModel(previewCtx, backend)
	previewModel.dashboard.Global = Scope{Available: true, Packs: []Pack{{ID: "argote", Surfaces: []string{"codex"}}}}
	previewModel.operation = "activate"
	_, previewCommand := previewModel.startPreview()
	previewDone := make(chan tea.Msg, 1)
	go func() { previewDone <- previewCommand() }()
	<-backend.previewStarted
	previewCancel()
	select {
	case message := <-previewDone:
		result := message.(previewResult)
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("preview error = %v; want cancellation", result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("preview remained blocked after cancellation")
	}
}

func TestApplyAndFreshReloadOutliveCanceledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	backend := &cancellationBackend{}
	model := newModel(ctx, backend)

	current, command := model.startApply(ApplyRequest{})
	current, firstInterrupt := current.Update(requestCanceled{})
	current, secondInterrupt := current.Update(requestCanceled{})
	if firstInterrupt != nil || secondInterrupt != nil {
		t.Fatal("repeated interrupts bypassed the non-interruptible Apply boundary")
	}
	batch, ok := command().(tea.BatchMsg)
	if !ok || len(batch) < 2 {
		t.Fatalf("Apply command = %#v; want operation and wait batch", command())
	}
	batch[0]()
	message := batch[1]()
	if _, ok := message.(applyProgressMessage); !ok {
		t.Fatalf("first Apply event = %#v; want progress", message)
	}
	current, wait := current.Update(message)
	finished := wait()
	if _, ok := finished.(applyFinished); !ok {
		t.Fatalf("second Apply event = %#v; want finish", finished)
	}
	current, reload := current.Update(finished)
	current, earlyQuit := current.Update(tea.KeyPressMsg(tea.Key{Text: "q", Code: 'q'}))
	if earlyQuit != nil {
		t.Fatal("quit completed before fresh post-Apply inspection")
	}
	loaded := reload()
	current, finalQuit := current.Update(loaded)
	if finalQuit == nil {
		t.Fatal("deferred quit did not complete after fresh post-Apply inspection")
	}

	if backend.applyContext == nil || backend.applyContext.Err() != nil {
		t.Fatalf("Apply context error = %v; want request-independent context", backend.applyContext.Err())
	}
	if backend.loadContextErr != nil {
		t.Fatalf("reload context error = %v; want request-independent context", backend.loadContextErr)
	}
	if !backend.loadHadDeadline {
		t.Fatal("fresh reload context has no independent deadline")
	}
}
