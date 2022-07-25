package tcp

import (
	"math"
	"time"
)

// A smoothed round-trip time struct.
type SRTT struct {
	alpha  float64
	beta   float64
	srtt   float64
	minRtt float64
	maxRtt float64
}

// Recommended: alpha = [0.8, 0.9], beta = [1.3, 2.0]
func NewSRTT(initialGuess, alpha, beta, minRtt, maxRtt float64) *SRTT {
	return &SRTT{
		alpha:  alpha,
		beta:   beta,
		srtt:   initialGuess,
		minRtt: minRtt,
		maxRtt: maxRtt,
	}
}

// Adds a measured rtt.
func (srtt *SRTT) AddPoint(rtt float64) {
	srtt.srtt = srtt.alpha*srtt.srtt + (1.0-srtt.alpha)*rtt
}

// Calculates what the rto should be.
func (srtt *SRTT) GetRTO() time.Duration {
	return time.Duration(math.Max(srtt.minRtt, math.Min(srtt.maxRtt, 3.0*srtt.beta*srtt.srtt)))
}
