package nethelper

import (
	"math/rand"
	"time"
)

func random(min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max-min) + min
}

// WaitForNextAttemp returns an integer that represents the seconds to wait for next attemp
func WaitForNextAttemp() time.Duration {
	return time.Duration(1)
}
