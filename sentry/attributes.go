package sentry

import "github.com/getsentry/sentry-go"

// Contains attributes which can be passed to glog, which will be used
// by this package to route and process Sentry errors accordingly.

type altDsn string

// AltDsn can be used as a glog attribute to specify a different DSN for the
// issue to be sent in Sentry
func AltDsn(dsn string) any {
	return altDsn(dsn)
}

type fingerprint []string

// Fingerprint creates a Sentry fingerprint from a variadic set of strings.
// This fingerprint will be added to the outgoing event to allow for custom rollup.
// See: https://docs.sentry.io/learn/rollups/#customize-grouping-with-fingerprints
func Fingerprint(print ...string) any {
	return fingerprint(print)
}

type sentryScope *sentry.Scope

// Scope can be used as a glog attribute to specify a scope to apply to the event sent to sentry
func Scope(scope *sentry.Scope) any {
	return sentryScope(scope)
}
