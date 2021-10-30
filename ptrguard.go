// Package ptrguard allows to pin a Go object (in memory allocated by the Go
// runtime), so that it will not be touched by the garbage collector until it is
// unpinned again.
package ptrguard

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"unsafe"
)

// Pinner can pin Go objects (in memory allocated by Go runtime) with the Pin()
// method. A pinned pointer to these objects can be stored in C memory
// (allocated by malloc) with the `Store()` method. All pinned objects of a
// Pinner can be unpinned with the `Unpin()` method.
type Pinner struct {
	*instance
}

// Pinned pointer that can be stored with the Store() method.
type Pinned struct {
	ptr  unsafe.Pointer
	data *data
}

// Pin the Go object referenced by pointer and return a Pinned value. The
// pointer can be a pointer of any type or unsafe.Pointer. The object will not
// be touched by the garbage collector until the `Unpin()` method is called.
// Therefore pinned pointers to this object can be directly stored in C memory
// with the `Store()` method or can be contained in Go memory passed to C
// functions, which usually violates the pointer passing rules[1].
//
// [1] https://golang.org/cmd/cgo/#hdr-Passing_pointers
func (p *Pinner) Pin(pointer interface{}) *Pinned {
	if p.instance == nil {
		p.instance = &instance{}
		runtime.SetFinalizer(p.instance, func(i *instance) {
			if i.data != nil {
				leakPanic()
			}
		})
	}
	if p.data == nil {
		p.data = &data{}
		p.release.Lock()
	}
	data := p.data
	ptr := getPtr(pointer)
	var pinned sync.Mutex
	pinned.Lock()
	// Start a background go routine that lives until Unpin() is called. This
	// calls a special function that makes sure the garbage collector doesn't
	// touch ptr and then waits until it receives the "release" signal, after
	// which it exits.
	data.wg.Add(1)
	go func() {
		pinUntilRelease(&pinned, &data.release, uintptr(ptr))
		data.wg.Done()
	}()
	pinned.Lock() // wait for the "pinned" signal from the go routine.
	return &Pinned{ptr, data}
}

// Unpin all pinned objects of the Pinner and zero all memory where the pointer
// has been stored. Whenever Pin() has been called at least once on a Pinner,
// Unpin() must be called afterwards on the same Pinner, or the garbage
// collector thread will panic.
func (p *Pinner) Unpin() {
	unpin(p.instance)
}

// Store a pinned pointer at target.
func (p *Pinned) Store(target interface{}) {
	ptrPtr := getPtrPtr(target)
	*hiddenPtr(ptrPtr) = *hiddenPtr(&p.ptr)
	p.data.add(ptrPtr)
}

// NoCheck temporarily disables cgocheck, which allows passing Go memory
// containing pinned Go pointers to a C function. Since this is a global
// setting, and if you are making C calls in parallel, theoretically it could
// happen that cgocheck is also disabled for some other C calls. If this is an
// issue, it is also possible to shadow the cgocheck call instead with this code
// line
//   _cgoCheckPointer := func(interface{}, interface{}) {}
// right before the C function call.
func NoCheck(f func()) {
	cgocheckOff()
	f()
	cgocheckOn()
}

type instance struct {
	*data
}

type data struct {
	release sync.RWMutex
	wg      sync.WaitGroup
	refs
}

func unpin(p *instance) {
	if p == nil || p.data == nil {
		return
	}
	p.refs.clear()
	p.release.Unlock() // broadcast "release" to all go routines
	p.wg.Wait()        // wait for all pinned pointers to be released
	p.data = nil
}

type refs struct {
	cPtr []*unsafe.Pointer
}

func (r *refs) add(target *unsafe.Pointer) {
	r.cPtr = append(r.cPtr, target)
}

func (r *refs) clear() {
	for i := range r.cPtr {
		*r.cPtr[i] = nil
		r.cPtr[i] = nil
	}
	r.cPtr = nil
}

var (
	cgocheckMtx sync.Mutex
	cgocheckCnt uint
	cgocheckOld int32
)

func cgocheckOff() {
	cgocheckMtx.Lock()
	if cgocheckCnt == 0 {
		cgocheckOld = *cgocheck
		*cgocheck = 0
	}
	cgocheckCnt++
	cgocheckMtx.Unlock()
}

func cgocheckOn() {
	cgocheckMtx.Lock()
	cgocheckCnt--
	if cgocheckCnt == 0 {
		*cgocheck = cgocheckOld
	}
	cgocheckMtx.Unlock()
}

func getPtr(i interface{}) unsafe.Pointer {
	val := reflect.ValueOf(i)
	if k := val.Kind(); k == reflect.Ptr || k == reflect.UnsafePointer {
		return unsafe.Pointer(val.Pointer())
	}
	panic(fmt.Sprintf("%s is not a pointer", val.Type()))
}

func getPtrPtr(i interface{}) *unsafe.Pointer {
	val := reflect.ValueOf(i)
	if k := val.Kind(); k == reflect.Ptr {
		if k = val.Elem().Kind(); k == reflect.Ptr || k == reflect.UnsafePointer {
			return (*unsafe.Pointer)(unsafe.Pointer(val.Pointer()))
		}
	}
	panic(fmt.Sprintf("%s is not a pointer to a pointer", val.Type()))
}

func hiddenPtr(p *unsafe.Pointer) *[unsafe.Sizeof(unsafe.Pointer(nil))]byte {
	return (*[unsafe.Sizeof(unsafe.Pointer(nil))]byte)(unsafe.Pointer(p))
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
	pinned.Unlock() // send "pinned" signal to main thread.
	release.RLock() // wait for "release" broadcast from main thread when
	//                 unpin() has been called.
}

// To be able to test that the GC panics when a pinned pointer is leaking, this
// panic function is a variable, that can be overwritten by a test.
var leakPanic = func() {
	panic("ptrguard: Found leaking pinned pointer. Forgot to call Unpin()?")
}
