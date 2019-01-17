package nethelper

import (
	"math/rand"
	"time"
)

func random(min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max-min) + min
}

// WaitForNextAttempt returns an integer that represents the seconds to wait for next attemp
func WaitForNextAttempt() time.Duration {
	return time.Duration(random(1, 5))
}
