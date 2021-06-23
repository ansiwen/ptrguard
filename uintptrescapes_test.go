package ptrguard

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func TestUintptrescapes(t *testing.T) {
	// This test assures that the special //go:uintptrescapes comment before
	// the storeUntilRelease() function works as intended, that is the
	// garbage collector doesn't touch the object referenced by the uintptr
	// until the function returns after Release() is called. The test will
	// fail if the //go:uintptrescapes comment is disabled (removed) or
	// stops working in future versions of go.
	var newPtr = func() (unsafe.Pointer, *bool) {
		var b bool
		s := make([]int, 1)
		runtime.SetFinalizer(&s, func(interface{}) { b = true })
		return unsafe.Pointer(&s), &b
	}
	for n := 0; n < 100; n++ {
		p1, p1Done := newPtr()
		p2, p2Done := newPtr()
		sync := make(syncCh)
		runtime.GC()
		assert.False(t, *p1Done)
		assert.False(t, *p2Done)
		var checkpoint bool
		go func() {
			pinUntilRelease(sync, uintptr(p1))
			checkpoint = true
			close(sync)
		}()
		<-sync
		assert.NotZero(t, p1)
		assert.NotZero(t, p2)
		p1 = nil
		p2 = nil
		assert.Zero(t, p1)
		assert.Zero(t, p2)
		runtime.GC()
		assert.False(t, *p1Done)
		assert.True(t, *p2Done)
		assert.False(t, checkpoint)
		sync <- signal
		<-sync
		assert.True(t, checkpoint)
		assert.False(t, *p1Done)
		runtime.GC()
		assert.True(t, *p1Done)
	}
}
