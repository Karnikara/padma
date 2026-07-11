package webhook

import (
	"math/rand/v2"
	"time"

	kwebhook "github.com/karnikara/kanaka/webhook"
)

// expJitter returns an exponential backoff with full jitter: attempt N waits a
// random duration in [d/2, d] where d = min(max, base * 2^(N-1)). Jitter spreads
// retries so a recovering receiver isn't hit by a synchronized thundering herd.
func expJitter(base, max time.Duration) kwebhook.BackoffFunc {
	return func(attempt int) time.Duration {
		d := base
		for i := 1; i < attempt; i++ {
			d *= 2
			if d >= max {
				d = max
				break
			}
		}
		half := d / 2
		if half <= 0 {
			return d
		}
		return half + time.Duration(rand.Int64N(int64(half)+1))
	}
}
