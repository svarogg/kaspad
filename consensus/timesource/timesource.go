package timesource

import (
	"github.com/kaspanet/kaspad/util/mstime"
)

// timeSource provides an implementation of the TimeSource interface
// that simply returns the current local time.
type timeSource struct{}

// Now returns the current local time, with one millisecond precision.
func (m *timeSource) Now() mstime.Time {
	return mstime.Now()
}

// New returns a new instance of a TimeSource
func New() *timeSource {
	return &timeSource{}
}
