package config

import "errors"

var (
	ErrNoSeedURLs         = errors.New("no seed URLs provided")
	ErrInvalidConcurrency = errors.New("concurrency must be greater than 0")
	ErrInvalidTimeout     = errors.New("request_timeout must be greater than 0")
	ErrEmptyDatabasePath  = errors.New("database_path cannot be empty")
)