package kiro

import "errors"

type kiroRequestError struct {
	code    string
	message string
}

func (err *kiroRequestError) Error() string { return err.message }

func newKiroRequestError(code, message string) error {
	return &kiroRequestError{code: code, message: message}
}

func kiroRequestErrorCode(err error) string {
	var typed *kiroRequestError
	if errors.As(err, &typed) && typed.code != "" {
		return typed.code
	}
	return "invalid_request"
}
