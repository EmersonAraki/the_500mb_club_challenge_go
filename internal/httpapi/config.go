package httpapi

import "time"

// Config holds HTTP-layer tunables.
type Config struct {
	// InstanceID is reported in the X-Instance-Id response header.
	InstanceID string
	// SingleMaxBytes caps the body of a single-point ingest.
	SingleMaxBytes int64
	// BatchMaxBytes caps the body of a batch ingest.
	BatchMaxBytes int64
	// ReadTimeout bounds each read-path store call so a stalled backend becomes
	// a fast 503 instead of holding a goroutine for the server WriteTimeout.
	ReadTimeout time.Duration
}
