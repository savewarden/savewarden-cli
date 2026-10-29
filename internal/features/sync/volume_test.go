package sync

import (
	"math"
	"testing"
)

func TestSpaceNeeded(t *testing.T) {
	for _, tt := range []struct {
		sizes []int64
		want  uint64
	}{
		{nil, minimumMargin},
		{[]int64{6}, 6 + minimumMargin},
		{[]int64{1 << 30, 1 << 30}, 2<<30 + (2<<30)/20 + 1},
		{[]int64{2000 << 20}, 2000<<20 + 100<<20},
	} {
		got, err := spaceNeeded(tt.sizes)
		if err != nil || got != tt.want {
			t.Errorf("%v: %d, %v; want %d", tt.sizes, got, err, tt.want)
		}
	}
	for _, sizes := range [][]int64{
		{math.MaxInt64, math.MaxInt64, math.MaxInt64},
		{math.MaxInt64, math.MaxInt64},
		{-1},
	} {
		if got, err := spaceNeeded(sizes); err == nil {
			t.Errorf("%v: %d accepted", sizes, got)
		}
	}
}
