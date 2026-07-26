package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/yersonargotev/packy/internal/opencodesmoke"
)

func main() {
	var c opencodesmoke.Config
	flag.StringVar(&c.OpenCode, "opencode", "", "exact acquired OpenCode executable")
	flag.StringVar(&c.SearchPath, "search-path", "", "explicit restricted subprocess PATH")
	flag.StringVar(&c.Version, "opencode-version", opencodesmoke.ExactVersion, "exact OpenCode version")
	flag.StringVar(&c.Integrity, "opencode-integrity", "", "official archive SHA256")
	flag.StringVar(&c.PackyRef, "packy-ref", "", "Packy ref")
	flag.StringVar(&c.PackySHA, "packy-sha", "", "Packy HEAD")
	flag.StringVar(&c.RunID, "run-id", "", "trusted workflow run ID")
	flag.StringVar(&c.EvidencePath, "evidence", "", "evidence JSON outside sandbox")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := opencodesmoke.Run(ctx, c); err != nil {
		fmt.Fprintln(os.Stderr, "opencodesmoke:", err)
		os.Exit(1)
	}
}
