package domain

import "errors"

var (
	ErrDuplicateProfile = errors.New("profile with the requested identifiers already exists")
	ErrInvalidData      = errors.New("invalid data provided for profile operations")
	ErrUnhandled        = errors.New("unexpected error")
	ErrProfileNotFound  = errors.New("profile not found")
)
