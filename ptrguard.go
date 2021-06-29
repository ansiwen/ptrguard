// Package ptrguard allows to pin a Go pointer (that is pointing to memory
// allocated by the Go runtime), so that the pointer will not be touched by the
// garbage collector until it is unpinned.
package ptrguard

import (
	"errors"
	"sync"
	"sync/atomic"
	"unsafe"
)

// Pinner is a PtrGuard scope context created by Scope() that can be used to pin
// Go pointers
type Pinner struct {
	*ctxData
}

// PinnedPtr is returned by Pin() and represents a pinned Go pointer (pointing
// to memory allocated by Go runtime) which can be stored in C memory (allocated
// by malloc) with the Poke() method.
type PinnedPtr struct {
	ptr    uintptr
	pinner Pinner
}

// Scope creates a PtrGuard scope by calling the provided function with a
// Pinner context. After the function returned it unpins all pinned pointers and
// resets all poked memory of that Pinner context to nil.
func Scope(f func(Pinner)) {
	ctx := ctxData{}
	ctx.release.Lock()
	f(Pinner{&ctx})
	ctx.clear()
}

// Pin pins a Go pointer within the PtrGuard scope of pinner and returns a
// PinnedPtr. The pointer will not be touched by the garbage collector until the
// end of the PtrGuard scope. Therefore it can be directly stored in C memory
// with the Poke() method or can be contained in Go memory passed to C
// functions, which usually violates the pointer passing rules[1].
//
// [1] https://golang.org/cmd/cgo/#hdr-Passing_pointers
func (pinner Pinner) Pin(ptr unsafe.Pointer) PinnedPtr {
	pinner.check()
	var pinned sync.Mutex
	pinned.Lock()
	// Start a background go routine that lives until Release() is called. This
	// calls a special function that makes sure the garbage collector doesn't
	// touch ptr and then waits until it receives the "release" signal, after
	// which it exits.
	pinner.wg.Add(1)
	go func() {
		pinUntilRelease(&pinned, &pinner.release, uintptr(ptr))
		pinner.wg.Done()
	}()
	pinned.Lock() // Wait for the "pinned" signal from the go routine <--(1)
	return PinnedPtr{uintptr(ptr), pinner}
}

// Poke stores the pinned pointer at target, which can be C memory. Target will
// be set to nil when the pointer is unpinned.
func (pinned PinnedPtr) Poke(target *unsafe.Pointer) {
	pinned.pinner.check()
	p := uintptrPtr(target)
	pinned.pinner.refs = append(pinned.pinner.refs, p)
	*p = uintptr(pinned.ptr)
}

// NoCheck temporarily disables cgocheck in order to pass Go memory containing
// pinned Go pointer to a C function. Since this is a global setting, and if you
// are making C calls in parallel, theoretically it could happen that cgocheck
// is also disabled for some other C calls. If this is an issue, it is possible
// to shadow the cgocheck call instead with this code line
//   _cgoCheckPointer := func(interface{}, interface{}) {}
// right before the C function call.
func (pinner Pinner) NoCheck(f func()) {
	pinner.check()
	cgocheckOld := atomic.SwapInt32(cgocheck, 0)
	f()
	atomic.StoreInt32(cgocheck, cgocheckOld)
}

// ErrInvalidPinner is thrown when invalid Pinners are accessed.
var ErrInvalidPinner = errors.New("access to invalid PtrGuard context")

type ctxData struct {
	release  sync.RWMutex
	wg       sync.WaitGroup
	refs     []*uintptr
	finished bool
}

func (ctx *ctxData) clear() {
	ctx.check()
	ctx.finished = true
	for i := range ctx.refs {
		*ctx.refs[i] = 0
		ctx.refs[i] = nil
	}
	ctx.refs = nil
	ctx.release.Unlock() // Broadcast "release" to all go routines. -->(2)
	ctx.wg.Wait()        // wait for all pinned pointers to be released
}

func (ctx *ctxData) check() {
	if ctx == nil || ctx.finished {
		panic(ErrInvalidPinner)
	}
}

// This code assumes that uintptr has the same size as a pointer, although in
// theory it could be larger.  Therefore we use this constant expression to
// assert size equality as a safeguard at compile time.
const _ = unsafe.Sizeof(unsafe.Pointer(nil)) - unsafe.Sizeof(uintptr(0))

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
func pinUntilRelease(pinned *sync.Mutex, release *sync.RWMutex, _ uintptr) {
	pinned.Unlock() // send "pinned" signal to main thread -->(1)
	release.RLock() // wait for "release" broadcast from main thread when
	//                 clear() has been called. <--(2)
}
