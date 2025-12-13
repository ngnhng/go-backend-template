package clock

import (
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
}

var RealClockProvider = sync.OnceValue(func() Clock {
	return &RealClock{}
})

type RealClock struct{}

func (RealClock) Now() time.Time {
	return time.Now()
}
