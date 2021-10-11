package ptrguard_test

import (
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/ansiwen/ptrguard"
	. "github.com/ansiwen/ptrguard/internal/testhelper"
	"github.com/stretchr/testify/assert"
)

const (
	ptrSize = unsafe.Sizeof(unsafe.Pointer(nil))
	fooBar  = "fooBar"
)

type tracer struct {
	p *string
	b *bool
}

func newTracer() tracer {
	var b bool
	s := "foobar"
	runtime.SetFinalizer(&s, func(interface{}) { b = true })
	return tracer{&s, &b}
}

func TestPin(t *testing.T) {
	tr1 := newTracer()
	tr2 := newTracer()
	cPtr := (*unsafe.Pointer)(Malloc(ptrSize))
	defer Free(unsafe.Pointer(cPtr))
	func() {
		var p ptrguard.Pinner
		defer p.Unpin()
		p.Pin(tr1.p).Store(cPtr)
		assert.Equal(t, unsafe.Pointer(tr1.p), *cPtr)
		tr1.p = nil
		tr2.p = nil
		runtime.GC()
		runtime.GC()
		assert.False(t, *tr1.b)
		assert.True(t, *tr2.b)
	}()
	runtime.GC()
	runtime.GC()
	assert.Eventually(t, func() bool { return *tr1.b == true },
		5*time.Second, 10*time.Millisecond)
	assert.Zero(t, *cPtr)
}

func TestReusePinner(t *testing.T) {
	tr1 := newTracer()
	tr2 := newTracer()
	cPtr := (*unsafe.Pointer)(Malloc(ptrSize))
	defer Free(unsafe.Pointer(cPtr))
	var p ptrguard.Pinner
	p.Pin(tr1.p).Store(cPtr)
	assert.Equal(t, unsafe.Pointer(tr1.p), *cPtr)
	tr1.p = nil
	runtime.GC()
	runtime.GC()
	assert.False(t, *tr1.b)
	p.Unpin()
	runtime.GC()
	runtime.GC()
	assert.Eventually(t, func() bool { return *tr1.b == true },
		5*time.Second, 10*time.Millisecond)
	assert.Zero(t, *cPtr)
	p.Pin(tr2.p).Store(cPtr)
	assert.Equal(t, unsafe.Pointer(tr2.p), *cPtr)
	tr2.p = nil
	runtime.GC()
	runtime.GC()
	assert.False(t, *tr2.b)
	p.Unpin()
	runtime.GC()
	runtime.GC()
	assert.Eventually(t, func() bool { return *tr2.b == true },
		5*time.Second, 10*time.Millisecond)
	assert.Zero(t, *cPtr)
}

func TestMultiStore(t *testing.T) {
	goPtr := &[1]byte{}
	cPtrArr := (*[1024]unsafe.Pointer)(Malloc(ptrSize * 1024))
	defer Free(unsafe.Pointer(&cPtrArr[0]))
	func() {
		var pg ptrguard.Pinner
		defer pg.Unpin()
		pp := pg.Pin(goPtr)
		for i := range cPtrArr {
			pp.Store(&cPtrArr[i])
		}
		for i := range cPtrArr {
			assert.Equal(t, cPtrArr[i], unsafe.Pointer(goPtr))
		}
	}()
	for i := range cPtrArr {
		assert.Zero(t, cPtrArr[i])
	}
}

func TestMultiPin(t *testing.T) {
	var trs [1024]tracer
	for i := range trs {
		trs[i] = newTracer()
	}
	func() {
		var pg ptrguard.Pinner
		defer pg.Unpin()
		for i := range trs {
			pg.Pin(trs[i].p)
			trs[i].p = nil
		}
		runtime.GC()
		runtime.GC()
		for i := range trs {
			assert.False(t, *trs[i].b)
		}
	}()
	runtime.GC()
	runtime.GC()
	assert.Eventually(t, func() bool { return *trs[len(trs)-1].b == true },
		5*time.Second, 10*time.Millisecond)
	for i := range trs {
		assert.True(t, *trs[i].b)
	}
}

func TestNoCheck(t *testing.T) {
	s := fooBar
	goPtr := (unsafe.Pointer)(&s)
	goPtrPtr := (unsafe.Pointer)(&goPtr)
	assert.Panics(t,
		func() {
			DummyCCall(goPtrPtr)
		},
		"Please run tests with GODEBUG=cgocheck=2",
	)
	assert.NotPanics(t,
		func() {
			ptrguard.NoCheck(func() {
				DummyCCall(goPtrPtr)
			})
		},
	)
	assert.Panics(t,
		func() {
			DummyCCall(goPtrPtr)
		},
		"Please run tests with GODEBUG=cgocheck=2",
	)
	assert.NotPanics(t,
		func() {
			ptrguard.NoCheck(func() {
				DummyCCall(goPtrPtr)
			})
		},
	)
}

func TestUnintialized(t *testing.T) {
	var pp ptrguard.Pinner
	assert.NotPanics(t,
		func() {
			pp.Unpin()
			pp.Unpin()
		},
	)
}

func TestDoubleUnpin(t *testing.T) {
	s := fooBar
	var pp ptrguard.Pinner
	pp.Pin(&s)
	assert.NotPanics(t,
		func() {
			pp.Unpin()
			pp.Unpin()
		},
	)
}

func TestNonPointerPanics(t *testing.T) {
	s := []byte("string")
	var pg ptrguard.Pinner
	assert.NotPanics(t,
		func() {
			pg.Pin(&s)
			pg.Unpin()
		},
	)
	assert.NotPanics(t,
		func() {
			pg.Pin(unsafe.Pointer(&s))
			pg.Unpin()
		},
	)
	assert.Panics(t,
		func() {
			pg.Pin(s)
		},
	)
}
