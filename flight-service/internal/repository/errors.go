package repository

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrResourceExhausted = errors.New("resource exhausted")
)
