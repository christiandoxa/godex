package usage

// Error marks a locally rejected command line as a parser failure. The
// composition root maps this boundary error to the conventional status 2.
type Error struct{ Cause error }

func (err Error) Error() string { return err.Cause.Error() }
func (err Error) Unwrap() error { return err.Cause }
func (Error) ExitCode() int     { return 2 }

func (Error) CLIArgumentError() bool { return true }

func Wrap(err error) error {
	if err == nil {
		return nil
	}
	return Error{Cause: err}
}
