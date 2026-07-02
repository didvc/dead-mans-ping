// Package metrics keeps a rolling summary of mouse movement over the last
// hour, day and week. Rather than storing every sample (which over a week
// would be millions of entries), it aggregates into fixed one-minute buckets
// held in a ring buffer of exactly one week. Memory use is therefore constant
// (~10k small buckets) regardless of how long the process runs.
package metrics

import (
	"sync"
	"time"
)

const (
	weekMinutes = 7 * 24 * 60 // 10080 one-minute buckets == one week
	hourMinutes = 60
	dayMinutes  = 24 * 60
)

// bucket accumulates movement for a single wall-clock minute. epochMin is the
// Unix-minute the bucket currently represents; -1 marks an unused bucket. When
// a bucket's slot is reused for a newer minute, its counters are reset, which
// is how week-old data is discarded without any explicit eviction pass.
type bucket struct {
	epochMin int64
	events   uint64
	distance float64
}

// Recorder is a concurrency-safe rolling movement recorder.
type Recorder struct {
	mu      sync.Mutex
	buckets []bucket
	now     func() time.Time
}

// New returns an empty Recorder.
func New() *Recorder {
	b := make([]bucket, weekMinutes)
	for i := range b {
		b[i].epochMin = -1
	}
	return &Recorder{buckets: b, now: time.Now}
}

func minuteOf(t time.Time) int64 { return t.Unix() / 60 }

// slot returns the bucket for epochMin, resetting it if it holds an older
// minute. Caller must hold r.mu.
func (r *Recorder) slot(epochMin int64) *bucket {
	idx := int(epochMin % weekMinutes)
	if idx < 0 {
		idx += weekMinutes
	}
	b := &r.buckets[idx]
	if b.epochMin != epochMin {
		b.epochMin = epochMin
		b.events = 0
		b.distance = 0
	}
	return b
}

// Record adds one movement observation covering the given pixel distance.
func (r *Recorder) Record(distance float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.slot(minuteOf(r.now()))
	b.events++
	b.distance += distance
}

// Window is an aggregate over one time span.
type Window struct {
	Events   uint64  // number of movement observations
	Distance float64 // total pixel distance travelled
}

// Summary holds the last-hour, last-day and last-week aggregates.
type Summary struct {
	Hour Window
	Day  Window
	Week Window
}

// Summary computes the current rolling aggregates. Cost is O(weekMinutes),
// i.e. a fixed ~10k iterations independent of runtime or sampling rate.
func (r *Recorder) Summary() Summary {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur := minuteOf(r.now())
	var s Summary
	for i := range r.buckets {
		b := &r.buckets[i]
		if b.epochMin < 0 {
			continue
		}
		age := cur - b.epochMin
		if age < 0 || age >= weekMinutes {
			continue
		}
		s.Week.Events += b.events
		s.Week.Distance += b.distance
		if age < dayMinutes {
			s.Day.Events += b.events
			s.Day.Distance += b.distance
		}
		if age < hourMinutes {
			s.Hour.Events += b.events
			s.Hour.Distance += b.distance
		}
	}
	return s
}
