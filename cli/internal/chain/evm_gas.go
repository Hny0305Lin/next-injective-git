package chain

import (
	"fmt"
	"math"
)

// AdjustEVMGasLimit computes floor(estimated*1.4)+headroom without allowing
// the intermediate multiplication to wrap uint64.
func AdjustEVMGasLimit(estimated, headroom uint64) (uint64, error) {
	quotient := estimated / 10
	remainder := estimated % 10
	if quotient > math.MaxUint64/14 {
		return 0, fmt.Errorf("estimated gas %d overflows gas adjustment", estimated)
	}
	scaled := quotient * 14
	extra := remainder * 14 / 10
	if scaled > math.MaxUint64-extra {
		return 0, fmt.Errorf("estimated gas %d overflows gas adjustment", estimated)
	}
	scaled += extra
	if scaled > math.MaxUint64-headroom {
		return 0, fmt.Errorf("estimated gas %d overflows gas adjustment", estimated)
	}
	return scaled + headroom, nil
}
