package providers

import (
	"strconv"
	"sync/atomic"
)

var idCounter uint64

func newID(prefix string) string {
	n := atomic.AddUint64(&idCounter, 1)
	return prefix + "-" + strconv.FormatUint(n, 10)
}
