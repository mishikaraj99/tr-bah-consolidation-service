package common

import "time"

// Clock abstracts time.Now for tests.
type Clock interface{ Now() time.Time }

// RealClock returns the current UTC time.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

// FixedClock always returns T.
type FixedClock struct{ T time.Time }

func (f FixedClock) Now() time.Time { return f.T }
