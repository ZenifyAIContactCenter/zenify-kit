//go:build tools

// Package deps pre-declares runtime dependencies that are added to go.mod
// ahead of the code that consumes them (onboarding TUI, milestone
// 2026-09-07-onboarding-tui-and-hooks). The build tag above excludes this
// file from every real build; its only purpose is to keep `go mod tidy`
// from pruning/marking these modules indirect before the TUI tasks land
// their own imports. Remove this file once those tasks import huh,
// bubbletea, and bubbles directly.
package deps

import (
	_ "github.com/charmbracelet/bubbles/textinput"
	_ "github.com/charmbracelet/bubbletea"
	_ "github.com/charmbracelet/huh"
)
