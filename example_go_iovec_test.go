package ptrguard_test

import (
	"fmt"
	"unsafe"

	"github.com/ansiwen/ptrguard"
	C "github.com/ansiwen/ptrguard/internal/testhelper"
)

// Example how to use PtrGuard with a Go allocated iovec slice.
func Example_goAllocatedIovec() {
	var buffers [][]byte
	for i := 2; i < 12; i += 3 {
		buffers = append(buffers, make([]byte, i))
	}

	iovec := make([]C.Iovec, len(buffers))

	var pinner ptrguard.Pinner
	defer pinner.Unpin()
	for i := range iovec {
		bufferPtr := &buffers[i][0]
		pinner.Pin(bufferPtr)
		iovec[i].Base = unsafe.Pointer(bufferPtr)
		iovec[i].Len = C.Int(len(buffers[i]))
	}

	ptrguard.NoCheck(func() {
		C.FillBuffersWithX(&iovec[0], len(iovec))
	})

	for i := range buffers {
		fmt.Println(string(buffers[i]))
	}
	// Output:
	// XX
	// XXXXX
	// XXXXXXXX
	// XXXXXXXXXXX
}
