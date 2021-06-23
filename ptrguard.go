package ptrguard

import (
	"sync"
	"unsafe"
)

type dbgVar struct {
	name  string
	value *int32
}

//go:linkname dbgvars runtime.dbgvars
var dbgvars []dbgVar

var cgocheck = func() *int32 {
	for i := range dbgvars {
		if dbgvars[i].name == "cgocheck" {
			return dbgvars[i].value
		}
	}
	panic("Couln't find cgocheck debug variable")
}()

type void [0]byte

type syncCh chan void

type metaData struct {
	sync syncCh
	refs []*uintptr
}

var (
	signal     void
	pinnedPtrs sync.Map
)

// The following code assumes that uintptr has the same size as a pointer,
// although in theory it could be larger.  Therefore we use this constant
// expression to assert size equality as a safeguard at compile time.
const _ = unsafe.Sizeof(unsafe.Pointer(nil)) - unsafe.Sizeof(uintptr(0))

// PinnedPtr respresents a pinned Go pointer (pointing to memory allocated by Go
// runtime) which can escape to C memory (allocated by malloc).
type PinnedPtr uintptr

// Pin the Go pointer ptr and return a PinnedPtr. The pointer will not be
// touched by the garbage collector until the Release() method has been called.
// Therefore it can be directly stored in C memory with the Poke() method or can
// be contained in Go memory passed to C functions, which usually violates the
// pointer passing rules[1].
// It's recommended to use a `defer pg.Release()` immediately after `pg :=
// Pin(...)` to avoid leaking resources and blocking the garbage collector.
// [1] https://golang.org/cmd/cgo/#hdr-Passing_pointers
func Pin(ptr unsafe.Pointer) PinnedPtr {
	sync := make(syncCh)
	// Start a background go routine that lives until Release() is called. This
	// calls a special function that makes sure the garbage collector doesn't
	// touch ptr and then waits until it receives the "release" signal, after
	// which it exits.
	go func() {
		pinUntilRelease(sync, uintptr(ptr))
		close(sync)
	}()
	// Wait for the "pinned" signal from the go routine <--(1)
	<-sync
	meta := &metaData{sync: sync}
	pp := PinnedPtr(ptr)
	pinnedPtrs.Store(pp, meta)
	return pp
}

// Poke stores the pinned pointer at target, which can be C memory. Target will
// be set to nil when Release() is called.
func (pp PinnedPtr) Poke(target *unsafe.Pointer) {
	v, ok := pinnedPtrs.Load(pp)
	if !ok {
		return
	}
	meta := v.(*metaData)
	p := uintptrPtr(target)
	meta.refs = append(meta.refs, p)
	*p = uintptr(pp)
}

// Release the pinned Go pointer. All poked targets will be reset to nil. The
// garbage collector will continue to manage the pointer as before it has been
// pinned.
func (pp PinnedPtr) Release() {
	v, ok := pinnedPtrs.LoadAndDelete(pp)
	if !ok {
		return
	}
	m := v.(*metaData)
	for i := range m.refs {
		*m.refs[i] = 0
		m.refs[i] = nil
	}
	m.refs = nil
	m.sync <- signal // Send the "release" signal to the go routine. -->(2)
	<-m.sync         // wait for Close()
}

// NoCgoCheck runs a code block with disabled cgocheck.
func NoCgoCheck(f func()) {
	before := *cgocheck
	*cgocheck = 0
	f()
	*cgocheck = before
}

func uintptrPtr(p *unsafe.Pointer) *uintptr {
	return (*uintptr)(unsafe.Pointer(p))
}

// From https://golang.org/src/cmd/compile/internal/gc/lex.go:
// For the next function declared in the file any uintptr arguments may be
// pointer values converted to uintptr. This directive ensures that the
// referenced allocated object, if any, is retained and not moved until the call
// completes, even though from the types alone it would appear that the object
// is no longer needed during the call. The conversion to uintptr must appear in
// the argument list.
// Also see https://golang.org/cmd/compile/#hdr-Compiler_Directives

//go:uintptrescapes
func pinUntilRelease(sync syncCh, _ uintptr) {
	sync <- signal // send "pinned" signal to main thread -->(1)
	<-sync         // wait for "release" signal from main thread when Release()
	//                has been called. <--(2)
}
