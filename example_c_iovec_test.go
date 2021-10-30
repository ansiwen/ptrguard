package ptrguard_test

import (
	"fmt"
	"math"

	"github.com/ansiwen/ptrguard"
	C "github.com/ansiwen/ptrguard/internal/testhelper"
)

// Example how to use PtrGuard with a C allocated iovec array.
func Example_cAllocatedIovec() {
	var buffers [][]byte
	for i := 2; i < 12; i += 3 {
		buffers = append(buffers, make([]byte, i))
	}
	numberOfBuffers := len(buffers)

	cPtr := C.Malloc(C.SizeOfIovec * uintptr(len(buffers)))
	defer C.Free(cPtr)
	// This is a trick to create a slice on top of the C allocated array, for
	// easier and safer access.
	iovec := (*[math.MaxInt32]C.Iovec)(cPtr)[:numberOfBuffers:numberOfBuffers]

	var pinner ptrguard.Pinner
	defer pinner.Unpin()
	for i := range iovec {
		bufferPtr := &buffers[i][0]
		pinner.Pin(bufferPtr).Store(&iovec[i].Base)
		iovec[i].Len = C.Int(len(buffers[i]))
	}

	C.FillBuffersWithX(&iovec[0], len(iovec))

	for i := range buffers {
		fmt.Println(string(buffers[i]))
	}
	// Output:
	// XX
	// XXXXX
	// XXXXXXXX
	// XXXXXXXXXXX
}
