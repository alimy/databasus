package restore

// A non-zero ExitCode is not a Go error here — only failing to create, start,
// or attach the exec is.
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// Result is populated even on the error path (see ErrRestoreFailed).
type Result struct {
	PgRestoreExitCode int
	DurationMs        int64
	StderrTail        string
}
