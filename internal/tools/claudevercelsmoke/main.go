package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/yersonargotev/packy/internal/claudesmoke"
)

func main() {
	var cfg claudesmoke.VercelConfig
	flag.StringVar(&cfg.Claude, "claude", "", "exact acquired Claude executable")
	flag.StringVar(&cfg.SearchPath, "search-path", "", "restricted executable search path")
	flag.StringVar(&cfg.ClaudeIntegrity, "claude-integrity", "", "npm dist.integrity for exact Claude package")
	flag.StringVar(&cfg.PackyRepo, "packy-repo", "", "Packy checkout")
	flag.StringVar(&cfg.PackyRef, "packy-ref", "", "Packy ref resolving to checkout HEAD")
	flag.StringVar(&cfg.EvidencePath, "evidence", "", "deterministic redacted JSON evidence")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := claudesmoke.RunVercel(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "claudevercelsmoke:", err)
		os.Exit(1)
	}
}
