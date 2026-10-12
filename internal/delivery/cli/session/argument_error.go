package session

type argumentError struct{ cause error }

func (err argumentError) Error() string      { return err.cause.Error() }
func (err argumentError) Unwrap() error      { return err.cause }
func (argumentError) ExitCode() int          { return 2 }
func (argumentError) CLIArgumentError() bool { return true }

func markArgumentError(err error) error {
	if err == nil {
		return nil
	}
	return argumentError{cause: err}
}
