package engine

// ring is a fixed-capacity circular buffer of live samples. Only the run
// goroutine writes; snapshot() is called under the engine mutex.
type ring struct {
	buf  []Sample
	next int
	full bool
}

func newRing(n int) *ring {
	if n < 1 {
		n = 1
	}
	return &ring{buf: make([]Sample, n)}
}

func (r *ring) push(s Sample) {
	r.buf[r.next] = s
	r.next++
	if r.next == len(r.buf) {
		r.next = 0
		r.full = true
	}
}

// snapshot returns the buffered samples in chronological order.
func (r *ring) snapshot() []Sample {
	if !r.full {
		out := make([]Sample, r.next)
		copy(out, r.buf[:r.next])
		return out
	}
	out := make([]Sample, 0, len(r.buf))
	out = append(out, r.buf[r.next:]...)
	out = append(out, r.buf[:r.next]...)
	return out
}
