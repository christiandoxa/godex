package runtime

// cliArgumentError distinguishes malformed CLI input from a correctly parsed
// command whose runtime cannot start. The tagged Prodex parser returns exit 2
// for syntax/option failures.
type cliArgumentError struct{ cause error }

func (err cliArgumentError) Error() string      { return err.cause.Error() }
func (err cliArgumentError) Unwrap() error      { return err.cause }
func (cliArgumentError) ExitCode() int          { return 2 }
func (cliArgumentError) CLIArgumentError() bool { return true }

type gatewayArgumentUsageError = cliArgumentError

func markCLIArgumentError(err error) error {
	if err == nil {
		return nil
	}
	return cliArgumentError{cause: err}
}
