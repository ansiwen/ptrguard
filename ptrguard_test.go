package ptrguard_test

import (
	"math/rand"
	"testing"
	"unsafe"

	"github.com/ansiwen/ptrguard"
	c "github.com/ansiwen/ptrguard/internal/test_c_helper"
	"github.com/stretchr/testify/assert"
)

const ptrSize = unsafe.Sizeof(unsafe.Pointer(nil))

func TestPinPokeRelease(t *testing.T) {
	s := "string"
	goPtr := (unsafe.Pointer)(&s)
	cPtrArr := (*[10]unsafe.Pointer)(c.Malloc(ptrSize * 10))
	defer c.Free(unsafe.Pointer(&cPtrArr[0]))
	pg := ptrguard.Pin(goPtr)
	for i := range cPtrArr {
		pg.Poke(&cPtrArr[i])
	}
	for i := range cPtrArr {
		assert.Equal(t, cPtrArr[i], goPtr)
	}
	pg.Release()
	for i := range cPtrArr {
		assert.Zero(t, cPtrArr[i])
	}
}

func TestMultiRelease(t *testing.T) {
	s := "string"
	goPtr := (unsafe.Pointer)(&s)
	cPtr := (*unsafe.Pointer)(c.Malloc(ptrSize))
	defer c.Free(unsafe.Pointer(cPtr))
	pg := ptrguard.Pin(goPtr)
	pg.Poke(cPtr)
	assert.Equal(t, *cPtr, goPtr)
	pg.Release()
	pg.Release()
	pg.Release()
	pg.Release()
	assert.Zero(t, *cPtr)
}

func TestNoCgoCheck(t *testing.T) {
	s := "string"
	goPtr := (unsafe.Pointer)(&s)
	goPtrPtr := (unsafe.Pointer)(&goPtr)
	assert.PanicsWithError(t,
		"runtime error: cgo argument has Go pointer to Go pointer",
		func() {
			c.DummyCCall(goPtrPtr)
		},
		"Please run tests with GODEBUG=cgocheck=2",
	)
	assert.NotPanics(t,
		func() {
			ptrguard.NoCgoCheck(func() {
				c.DummyCCall(goPtrPtr)
			})
		},
	)
}

func TestStressTest(t *testing.T) {
	// Because the default thread limit of the Go runtime is 10000, creating
	// 20000 parallel PtrGuards asserts, that Go routines of PtrGuards don't
	// create threads.
	const N = 20000  // Number of parallel PtrGuards
	const M = 100000 // Number of loops
	var ptrGuards [N]ptrguard.PinnedPtr
	cPtrArr := (*[N]unsafe.Pointer)(c.Malloc(N * ptrSize))
	defer c.Free(unsafe.Pointer(&cPtrArr[0]))
	toggle := func(i int) {
		if ptrGuards[i] == 0 {
			goPtr := unsafe.Pointer(&(struct{ byte }{42}))
			cPtrPtr := unsafe.Pointer(&cPtrArr[i])
			ptrGuards[i] = ptrguard.Pin(goPtr)
			ptrGuards[i].Poke((*unsafe.Pointer)(cPtrPtr))
			assert.Equal(t, (unsafe.Pointer)(cPtrArr[i]), goPtr)
		} else {
			ptrGuards[i].Release()
			ptrGuards[i] = 0
			assert.Zero(t, cPtrArr[i])
		}
	}
	for i := range ptrGuards {
		toggle(i)
	}
	for n := 0; n < M; n++ {
		i := rand.Intn(N)
		toggle(i)
	}
	for i := range ptrGuards {
		if ptrGuards[i] != 0 {
			ptrGuards[i].Release()
			ptrGuards[i] = 0
		}
	}
	for i := uintptr(0); i < N; i++ {
		assert.Zero(t, cPtrArr[i])
	}
}
