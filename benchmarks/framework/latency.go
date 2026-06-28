package framework

import (
	"math"
	"sort"
	"sync"
)

type Histogram struct {
	mu   sync.Mutex
	data []float64
}

func NewHistogram() *Histogram {
	return &Histogram{data: make([]float64, 0, 1024)}
}

func (h *Histogram) Observe(v float64) {
	h.mu.Lock()
	h.data = append(h.data, v)
	h.mu.Unlock()
}

func (h *Histogram) Percentile(p float64) float64 {
	h.mu.Lock()
	d := append([]float64(nil), h.data...)
	h.mu.Unlock()
	if len(d) == 0 {
		return 0
	}
	sort.Float64s(d)
	idx := int(math.Ceil((p/100)*float64(len(d)))) - 1
	if idx < 0 {
		idx = 0
	}
	return d[idx]
}

func (h *Histogram) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.data)
}

func (h *Histogram) Mean() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.data) == 0 {
		return 0
	}
	var sum float64
	for _, v := range h.data {
		sum += v
	}
	return sum / float64(len(h.data))
}

func (h *Histogram) Max() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.data) == 0 {
		return 0
	}
	m := h.data[0]
	for _, v := range h.data {
		if v > m {
			m = v
		}
	}
	return m
}

// Stdev returns the population standard deviation of the observed samples.
func (h *Histogram) Stdev() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := len(h.data)
	if n == 0 {
		return 0
	}
	var sum, mean float64
	for _, v := range h.data {
		sum += v
	}
	mean = sum / float64(n)
	var sq float64
	for _, v := range h.data {
		d := v - mean
		sq += d * d
	}
	return math.Sqrt(sq / float64(n))
}
