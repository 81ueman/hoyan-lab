package cli

import "errors"

type ExitError struct {
	Code int
	Err  error
}

func (e ExitError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func asExitError(err error, target *ExitError) bool {
	return errors.As(err, target)
}
