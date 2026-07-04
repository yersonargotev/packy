package screens

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestWrapWelcomeBanner_UsesDisplayWidthForWideAdvisory(t *testing.T) {
	const width = 40
	text := "Advisory: 🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀 deployment notice must stay inside the welcome frame"

	wrapped := wrapWelcomeBanner(text, width)

	for i, line := range strings.Split(wrapped, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("wrapped line %d display width = %d, want <= %d\nline: %q\nwrapped:\n%s", i, got, width, line, wrapped)
		}
	}
}

func TestWrapWelcomeBanner_FormatsRecognizableAdvisoryAsList(t *testing.T) {
	const width = 72
	text := "Advisory: 🚀 Big gentle-ai update: SDD now self-validates every phase. Plus: engram survives brew upgrades, Windows install fixes land, and a revamped updater detects beta commits. Thanks for testing. See https://example.test/release"

	wrapped := wrapWelcomeBanner(text, width)

	for _, want := range []string{
		"Advisory: 🚀 Big gentle-ai update:",
		"• SDD now self-validates every phase.",
		"• engram survives brew upgrades",
		"• Windows install fixes land",
		"• a revamped updater detects beta commits.",
	} {
		if !strings.Contains(wrapped, want) {
			t.Fatalf("wrapped advisory missing %q\nwrapped:\n%s", want, wrapped)
		}
	}

	for i, line := range strings.Split(wrapped, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("wrapped line %d display width = %d, want <= %d\nline: %q\nwrapped:\n%s", i, got, width, line, wrapped)
		}
	}
}

func TestWrapWelcomeBanner_PreservesPlainTextAdvisory(t *testing.T) {
	text := "Advisory: maintenance window tonight"

	if got := wrapWelcomeBanner(text, 80); got != text {
		t.Fatalf("wrapWelcomeBanner() = %q, want %q", got, text)
	}
}
