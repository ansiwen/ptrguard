package ptrguard // nolint:testpackage

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLeakPanics(t *testing.T) {
	assert.Panics(t, leakPanic)
	leaked := false
	leakPanic = func() {
		leaked = true
	}
	func() {
		var pg Pinner
		pg.Pin(&[1]byte{})
	}()
	runtime.GC()
	runtime.GC()
	assert.Eventually(t, func() bool { return leaked == true },
		5*time.Second, 10*time.Millisecond)
}
