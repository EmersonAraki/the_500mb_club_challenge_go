package httpapi

// Config holds HTTP-layer tunables.
type Config struct {
	// InstanceID is reported in the X-Instance-Id response header.
	InstanceID string
	// SingleMaxBytes caps the body of a single-point ingest.
	SingleMaxBytes int64
	// BatchMaxBytes caps the body of a batch ingest.
	BatchMaxBytes int64
}
