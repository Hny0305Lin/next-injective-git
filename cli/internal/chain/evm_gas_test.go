package chain

import (
	"math"
	"strings"
	"testing"
)

func TestAdjustEVMGasLimitUsesExactFloorAndHeadroom(t *testing.T) {
	for _, tc := range []struct {
		estimated uint64
		headroom  uint64
		want      uint64
	}{
		{estimated: 0, headroom: 10_000, want: 10_000},
		{estimated: 21_000, headroom: 10_000, want: 39_400},
		{estimated: 21_003, headroom: 10_000, want: 39_404},
	} {
		got, err := AdjustEVMGasLimit(tc.estimated, tc.headroom)
		if err != nil || got != tc.want {
			t.Fatalf("AdjustEVMGasLimit(%d, %d) = %d, %v; want %d", tc.estimated, tc.headroom, got, err, tc.want)
		}
	}
}

func TestAdjustEVMGasLimitRejectsTrueOverflowWithoutIntermediateWrap(t *testing.T) {
	// This value made the previous gas*14/10 expression wrap even though its
	// mathematical final value still fits uint64.
	nearBoundary := (uint64(math.MaxUint64) - 10_000) / 14 * 10
	adjusted, err := AdjustEVMGasLimit(nearBoundary, 10_000)
	if err != nil || adjusted <= nearBoundary {
		t.Fatalf("near-boundary adjustment = %d, %v", adjusted, err)
	}
	if _, err := AdjustEVMGasLimit(math.MaxUint64, 10_000); err == nil || !strings.Contains(err.Error(), "overflows gas adjustment") {
		t.Fatalf("overflow error = %v", err)
	}
}
