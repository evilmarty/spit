package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

var (
	BuildDate = "unknown"
	Version   = "dev"
	Commit    = "none"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runWithContext(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitCodeForError(err))
	}
}

func exitCodeForError(err error) int {
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if errors.Is(err, errInterrupted) {
		return 130
	}
	return 1
}
