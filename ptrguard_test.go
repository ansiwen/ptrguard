package ptrguard_test

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/ansiwen/ptrguard"
	c "github.com/ansiwen/ptrguard/internal/test_c_helper"
	"github.com/stretchr/testify/assert"
)

const ptrSize = unsafe.Sizeof(unsafe.Pointer(nil))

type tracer struct {
	p unsafe.Pointer
	b *bool
}

func newTracer() tracer {
	var b bool
	s := make([]int, 1)
	runtime.SetFinalizer(&s, func(interface{}) { b = true })
	return tracer{unsafe.Pointer(&s), &b}
}

func TestPinPoke(t *testing.T) {
	tr1 := newTracer()
	tr2 := newTracer()
	cPtr := (*unsafe.Pointer)(c.Malloc(ptrSize))
	defer c.Free(unsafe.Pointer(cPtr))
	ptrguard.Scope(func(pg ptrguard.Pinner) {
		pg.Pin(tr1.p).Poke(cPtr)
		assert.Equal(t, tr1.p, *cPtr)
		tr1.p = nil
		tr2.p = nil
		runtime.GC()
		runtime.GC()
		assert.False(t, *tr1.b)
		assert.True(t, *tr2.b)
	})
	runtime.GC()
	runtime.GC()
	assert.True(t, *tr1.b)
	assert.Zero(t, *cPtr)
}

func TestMultiPoke(t *testing.T) {
	goPtr := (unsafe.Pointer)(&[1]byte{})
	cPtrArr := (*[1024]unsafe.Pointer)(c.Malloc(ptrSize * 1024))
	defer c.Free(unsafe.Pointer(&cPtrArr[0]))
	ptrguard.Scope(func(pg ptrguard.Pinner) {
		pp := pg.Pin(goPtr)
		for i := range cPtrArr {
			pp.Poke(&cPtrArr[i])
		}
		for i := range cPtrArr {
			assert.Equal(t, cPtrArr[i], goPtr)
		}
	})
	for i := range cPtrArr {
		assert.Zero(t, cPtrArr[i])
	}
}

func TestMultiPin(t *testing.T) {
	var trs [1024]tracer
	for i := range trs {
		trs[i] = newTracer()
	}
	ptrguard.Scope(func(pg ptrguard.Pinner) {
		for i := range trs {
			pg.Pin(trs[i].p)
			trs[i].p = nil
		}
		runtime.GC()
		runtime.GC()
		for i := range trs {
			assert.False(t, *trs[i].b)
		}
	})
	runtime.GC()
	runtime.GC()
	for i := range trs {
		assert.True(t, *trs[i].b)
	}
}

func TestNoCheck(t *testing.T) {
	s := "string"
	goPtr := (unsafe.Pointer)(&s)
	goPtrPtr := (unsafe.Pointer)(&goPtr)
	assert.Panics(t,
		func() {
			c.DummyCCall(goPtrPtr)
		},
		"Please run tests with GODEBUG=cgocheck=2",
	)
	assert.NotPanics(t,
		func() {
			ptrguard.Scope(func(pg ptrguard.Pinner) {
				pg.NoCheck(func() {
					c.DummyCCall(goPtrPtr)
				})
			})
		},
	)
}

func TestOutOfScopePanics(t *testing.T) {
	s := "string"
	goPtr := (unsafe.Pointer)(&s)
	var goPtrPtr *unsafe.Pointer
	var pg ptrguard.Pinner
	var pp ptrguard.PinnedPtr
	ptrguard.Scope(func(ctx ptrguard.Pinner) {
		pg = ctx
		pp = pg.Pin(goPtr)
	})
	assert.PanicsWithValue(t,
		ptrguard.ErrInvalidPinner,
		func() {
			pg.Pin(goPtr)
		},
	)
	assert.PanicsWithValue(t,
		ptrguard.ErrInvalidPinner,
		func() {
			pp.Poke(goPtrPtr)
		},
	)
}

func TestUnintializedPanics(t *testing.T) {
	s := "string"
	goPtr := (unsafe.Pointer)(&s)
	var goPtrPtr *unsafe.Pointer
	var pg ptrguard.Pinner
	var pp ptrguard.PinnedPtr
	assert.PanicsWithValue(t,
		ptrguard.ErrInvalidPinner,
		func() {
			pg.Pin(goPtr)
		},
	)
	assert.PanicsWithValue(t,
		ptrguard.ErrInvalidPinner,
		func() {
			pp.Poke(goPtrPtr)
		},
	)
}
