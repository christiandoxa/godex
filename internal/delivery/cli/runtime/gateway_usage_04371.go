package runtime

// gatewayArgumentUsageError distinguishes malformed CLI input from a
// correctly parsed gateway whose runtime cannot start. The tagged Prodex
// 0.437.1 command parser returns exit 2 for syntax/option failures.
type gatewayArgumentUsageError struct{ cause error }

func (err gatewayArgumentUsageError) Error() string      { return err.cause.Error() }
func (err gatewayArgumentUsageError) Unwrap() error      { return err.cause }
func (gatewayArgumentUsageError) ExitCode() int          { return 2 }
func (gatewayArgumentUsageError) CLIArgumentError() bool { return true }
