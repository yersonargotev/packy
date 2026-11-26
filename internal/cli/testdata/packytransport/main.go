package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/yersonargotev/packy/internal/cli"
	"github.com/yersonargotev/packy/internal/githubfake"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	client := githubfake.NewClient(os.Getenv("PACKY_GITHUBFAKE_ADDR_FILE"))
	command := cli.NewRootCommand(cli.Options{HTTPClient: client, AttestationVerifier: githubfake.NewAttestationVerifier(client)})
	err := command.ExecuteContext(ctx)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
