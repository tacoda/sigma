package eval

import "math"

const signEps = 1e-9

// signTest classifies paired deltas into wins (B>A), losses (B<A), and ties,
// and returns the two-sided sign-test p-value over the non-tie pairs.
func signTest(deltas []float64) (wins, losses, ties int, p float64) {
	for _, d := range deltas {
		switch {
		case d > signEps:
			wins++
		case d < -signEps:
			losses++
		default:
			ties++
		}
	}
	n := wins + losses
	if n == 0 {
		return wins, losses, ties, 1
	}
	k := wins
	if losses < k {
		k = losses
	}
	return wins, losses, ties, binomTwoSided(k, n)
}

// binomTwoSided is the two-sided tail probability of Binomial(n, 0.5) at k.
func binomTwoSided(k, n int) float64 {
	var tail float64
	for i := 0; i <= k; i++ {
		tail += choose(n, i) * math.Pow(0.5, float64(n))
	}
	if p := 2 * tail; p < 1 {
		return p
	}
	return 1
}

// choose(n, k) as a float, computed iteratively to avoid overflow.
func choose(n, k int) float64 {
	if k < 0 || k > n {
		return 0
	}
	if k > n-k {
		k = n - k
	}
	c := 1.0
	for i := 0; i < k; i++ {
		c = c * float64(n-i) / float64(i+1)
	}
	return c
}
