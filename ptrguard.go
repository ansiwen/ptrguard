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

// PinnedPtr represents a pinned Go pointer (pointing to memory allocated by Go
// runtime) which can be stored in C memory (allocated by malloc) with the
// Poke() method and unpinned with the Unpin() method.
type PinnedPtr struct {
	ptr     uintptr
	release sync.Mutex
	pinned  sync.Mutex
	refsType
	finishedType
}

// Pin pins a Go pointer and returns a PinnedPtr. The pointer will not be
// touched by the garbage collector until Unpin() is called. Therefore it can be
// directly stored in C memory with the Poke() method or can be contained in Go
// memory passed to C functions, which usually violates the pointer passing
// rules[1].
//
// [1] https://golang.org/cmd/cgo/#hdr-Passing_pointers
func Pin(ptr unsafe.Pointer) *PinnedPtr {
	var p PinnedPtr
	p.release.Lock()
	p.pinned.Lock()
	// Start a background go routine that lives until Release() is called. This
	// calls a special function that makes sure the garbage collector doesn't
	// touch ptr and then waits until it receives the "release" signal, after
	// which it exits.
	go func() {
		pinUntilRelease(&p.pinned, &p.release, uintptr(ptr))
		p.pinned.Unlock() // send "released" signal.
	}()
	p.pinned.Lock() // wait for the "pinned" signal from the go routine.
	p.ptr = uintptr(ptr)
	return &p
}

// Poke stores the pinned pointer at target, which can be C memory. Target will
// be set to nil when the pointer is unpinned.
func (p *PinnedPtr) Poke(target *unsafe.Pointer) *PinnedPtr {
	p.check()
	p.poke(p.ptr, target)
	return p
}

// Unpin unpins the pinned pointer and resets all poked memory of that pinned
// pointer context to nil.
func (p *PinnedPtr) Unpin() {
	p.check()
	p.finished = true
	p.clear()
	p.release.Unlock() // send "release" signal to go routine.
	p.pinned.Lock()    // wait for "released" signal.
}

// Pinner is a PtrGuard scope context created by Scope() that can be used to pin
// Go pointers
type Pinner struct {
	*pinnerData
}

// ScopedPinnedPtr represents a scoped pinned Go pointer (pointing to memory
// allocated by Go runtime) which can be stored in C memory (allocated by
// malloc) with the Poke() method and will be unpinned when the scope is left.
type ScopedPinnedPtr struct {
	ptr    uintptr
	pinner Pinner
}

// Scope creates a PtrGuard scope by calling the provided function with a
// Pinner context. After the function returned it unpins all pinned pointers and
// resets all poked memory of that Pinner context to nil.
func Scope(f func(Pinner)) {
	var pinner pinnerData
	pinner.release.Lock()
	f(Pinner{&pinner})
	pinner.unpin()
}

// Pin pins a Go pointer within the PtrGuard scope of pinner and returns a
// PinnedPtr. The pointer will not be touched by the garbage collector until the
// end of the PtrGuard scope. Therefore it can be directly stored in C memory
// with the Poke() method or can be contained in Go memory passed to C
// functions, which usually violates the pointer passing rules[1].
//
// [1] https://golang.org/cmd/cgo/#hdr-Passing_pointers
func (v Pinner) Pin(ptr unsafe.Pointer) ScopedPinnedPtr {
	v.check()
	var pinned sync.Mutex
	pinned.Lock()
	// Start a background go routine that lives until Release() is called. This
	// calls a special function that makes sure the garbage collector doesn't
	// touch ptr and then waits until it receives the "release" signal, after
	// which it exits.
	v.wg.Add(1)
	go func() {
		pinUntilRelease2(&pinned, &v.release, uintptr(ptr))
		v.wg.Done()
	}()
	pinned.Lock() // wait for the "pinned" signal from the go routine.
	return ScopedPinnedPtr{uintptr(ptr), v}
}

// Poke stores the pinned pointer at target, which can be C memory. Target will
// be set to nil when the pointer is unpinned.
func (pinned ScopedPinnedPtr) Poke(target *unsafe.Pointer) ScopedPinnedPtr {
	pinned.pinner.check()
	pinned.pinner.poke(pinned.ptr, target)
	return pinned
}

// NoCheck temporarily disables cgocheck in order to pass Go memory containing
// pinned Go pointer to a C function. Since this is a global setting, and if you
// are making C calls in parallel, theoretically it could happen that cgocheck
// is also disabled for some other C calls. If this is an issue, it is possible
// to shadow the cgocheck call instead with this code line
//   _cgoCheckPointer := func(interface{}, interface{}) {}
// right before the C function call.
func NoCheck(f func()) {
	cgocheckOld := atomic.SwapInt32(cgocheck, 0)
	f()
	atomic.StoreInt32(cgocheck, cgocheckOld)
}

// ErrInvalidPinner is thrown when invalid Pinners are accessed.
var ErrInvalidPinner = errors.New("access to invalid PtrGuard context")

type pinnerData struct {
	release sync.RWMutex
	wg      sync.WaitGroup
	refsType
	finishedType
}

func (v *pinnerData) unpin() {
	v.check()
	v.finished = true
	v.clear()
	v.release.Unlock() // broadcast "release" to all go routines
	v.wg.Wait()        // wait for all pinned pointers to be released
}

type finishedType struct {
	finished bool
}

func (v *finishedType) check() {
	if v.finished {
		panic(ErrInvalidPinner)
	}
}

type refsType struct {
	refs []*uintptr
}

func (v *refsType) poke(ptr uintptr, target *unsafe.Pointer) {
	uip := uintptrPtr(target)
	v.refs = append(v.refs, uip)
	*uip = ptr
}

func (v *refsType) clear() {
	for i := range v.refs {
		*v.refs[i] = 0
		v.refs[i] = nil
	}
	v.refs = nil
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
func pinUntilRelease(pinned *sync.Mutex, release *sync.Mutex, _ uintptr) {
	pinned.Unlock() // send "pinned" signal to main thread -->(1)
	release.Lock()  // wait for "release" signal from main thread when
	//                 Unpin() has been called. <--(2)
}

//go:uintptrescapes
func pinUntilRelease2(pinned *sync.Mutex, release *sync.RWMutex, _ uintptr) {
	pinned.Unlock() // send "pinned" signal to main thread -->(1)
	release.RLock() // wait for "release" broadcast from main thread when
	//                 clear() has been called. <--(2)
}
