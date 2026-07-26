package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/yersonargotev/packy/internal/codexsmoke"
	"os"
	"time"
)

func main() {
	var c codexsmoke.Config
	flag.StringVar(&c.Codex, "codex", "", "exact acquired Codex executable")
	flag.StringVar(&c.SearchPath, "search-path", "", "explicit restricted subprocess PATH")
	flag.StringVar(&c.Version, "codex-version", codexsmoke.ExactFloor, "exact Codex version")
	flag.StringVar(&c.Integrity, "codex-integrity", "", "npm dist.integrity")
	flag.StringVar(&c.PackyRef, "packy-ref", "", "Packy ref")
	flag.StringVar(&c.PackySHA, "packy-sha", "", "Packy HEAD")
	flag.StringVar(&c.RunID, "run-id", "", "trusted workflow run ID")
	flag.StringVar(&c.EvidencePath, "evidence", "", "evidence JSON outside sandbox")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, e := codexsmoke.Run(ctx, c); e != nil {
		fmt.Fprintln(os.Stderr, "codexsmoke:", e)
		os.Exit(1)
	}
}
