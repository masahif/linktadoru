package config

import "errors"

var (
	// ErrInvalidConcurrency is returned when concurrency is not greater than 0
	ErrInvalidConcurrency = errors.New("concurrency must be greater than 0")
	// ErrInvalidTimeout is returned when request timeout is not greater than 0
	ErrInvalidTimeout = errors.New("request_timeout must be greater than 0")
	// ErrEmptyDatabasePath is returned when database path is empty
	ErrEmptyDatabasePath = errors.New("database_path cannot be empty")
)
