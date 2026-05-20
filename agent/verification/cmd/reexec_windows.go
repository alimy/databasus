//go:build windows

package main

import (
	"log/slog"
	"os"
)

func reexecAfterUpgrade(log *slog.Logger) {
	log.Error("self-update re-exec is not supported on Windows; run the verification agent on Linux")
	os.Exit(1)
}
