package errors

import "errors"

var (
	ErrConstraintNotSatisfied   = errors.New("no available version satisfies the constraint")
	ErrEnvNotSet                = errors.New("environment variable not set")
	ErrInvalidKubeVersionFormat = errors.New("invalid KubeVersion format: expected Major.Minor(.ANY)")
	ErrNotFound                 = errors.New("not found")
	ErrUnexpectedType           = errors.New("expected a map for key")
)
