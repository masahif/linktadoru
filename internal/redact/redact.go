// Package redact renders configuration values that may carry credentials in a
// form safe to print. Diagnostics, validation errors, and logs all quote the
// input that caused them, so each of those sites needs the same rendering — a
// single implementation here keeps them from drifting apart.
package redact

import (
	"net/url"
	"strings"
)

// Value is the marker substituted for a secret. It is stable so that output
// can be asserted on and so operators can recognize it.
const Value = "<redacted>"

// URL removes userinfo credentials from raw, keeping the rest of the URL so it
// remains recognizable. A URL that cannot be parsed is replaced entirely,
// because there is no way to tell which part of it is a credential.
//
// Credentials carried in a query string are not removed; deciding what a query
// parameter is allowed to contain is a separate policy question.
func URL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return Value
	}
	if parsed.User == nil {
		return raw
	}
	parsed.User = url.User("redacted")
	return parsed.String()
}

// Header replaces the value of a "Name: Value" header with the marker, keeping
// the name. A header that does not carry a usable name is replaced entirely,
// since the whole string may then be the secret.
func Header(header string) string {
	name, value, found := strings.Cut(header, ":")
	if !found {
		return Value
	}
	if strings.TrimSpace(name) == "" {
		return ": " + Value
	}
	if strings.TrimSpace(value) == "" {
		return strings.TrimSpace(name) + ":"
	}
	return strings.TrimSpace(name) + ": " + Value
}
