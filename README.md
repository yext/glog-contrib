glog-contrib
============

Contributed code for use with the glog package.  For example, logging backends that integrate with other services.

## Sentry

The Sentry package contains an implementation of a bridge between
the sentry-go package and glog which allows for glog errors of
level ERROR to be tracked as errors in Sentry. This includes
custom code that interfaces with the xerrors package (as well
as the Yext fork named yerrors) in order to send stack trace
data for the glog invocation, error object construction,
and any masked error calls to Sentry.

`sentry.CaptureErrors` is the entrypoint for tracking Sentry exceptions via glog.
Given Sentry DSNs and client options (DSN should not be specified in opts),
constructs individual Sentry Client's for each DSN. The glog.Event channel
should be provided by running `glog.RegisterBackend()`. For example:

```go
sentry.CaptureErrors(
  "projectName",
  []string{"https://primaryDsn", "https://optionalSecondaryDsn", ...},
  sentrygo.ClientOptions{
    Release: "release",
    Environment: "prod",
  },
  glog.RegisterBackend())
```

When an event is received via glog at the ERROR severity,
the first provided DSN will be used, unless a `sentry.AltDsn`
is tagged on the glog event, in which case the specified client
for that DSN will be used:

```go
glog.Error("error for secondary DSN", sentry.AltDsn("https://optionalSecondaryDsn"))
```

## Installation
glog-contrib is released as a Go module. To download the latest version, run
```
go get github.com/yext/glog-contrib
```
