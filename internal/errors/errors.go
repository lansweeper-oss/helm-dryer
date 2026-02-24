package errors

import "errors"

var (
	ErrEnvNotSet                = errors.New("environment variable not set")
	ErrInvalidKubeVersionFormat = errors.New("invalid KubeVersion format: expected Major.Minor(.ANY)")
	ErrUnexpectedType           = errors.New("expected a map for key")
)
