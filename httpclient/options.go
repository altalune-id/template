package httpclient

import "time"

// Option configures a client built by New or NewProber.
type Option func(*settings)

type settings struct {
	allowPrivateHosts bool
	dialTimeout       time.Duration
	timeout           time.Duration
	responseBodyLimit int64
	otel              bool
}

// WithAllowPrivateHosts turns the SSRF dial filter off for trusted, operator-configured URLs.
func WithAllowPrivateHosts(allow bool) Option {
	return func(s *settings) { s.allowPrivateHosts = allow }
}

// WithDialTimeout bounds each dial attempt.
func WithDialTimeout(d time.Duration) Option {
	return func(s *settings) { s.dialTimeout = d }
}

// WithTimeout bounds the whole request, including redirects and body reads.
func WithTimeout(d time.Duration) Option {
	return func(s *settings) { s.timeout = d }
}

// WithResponseBodyLimit caps how many bytes a response body may yield; zero means unlimited.
func WithResponseBodyLimit(n int64) Option {
	return func(s *settings) { s.responseBodyLimit = n }
}

// WithOtel toggles otelhttp transport instrumentation.
func WithOtel(enabled bool) Option {
	return func(s *settings) { s.otel = enabled }
}
