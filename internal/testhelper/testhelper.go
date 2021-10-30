// package testhelper
package testhelper

/*
#include <stdlib.h>

void dummyCall(void* p) {}

typedef struct {
	void* Base;
	int Len;
} iovec;

inline void fillBufsWithX(iovec* bufs, int n) {
	for (int i = 0; i<n; ++i) {
		for (int j = 0; j<bufs[i].Len ; ++j) {
			((char*)(bufs[i].Base))[j] = 'X';
		}
	}
}
*/
import "C"

import (
	"unsafe"
)

// Iovec ...
type Iovec C.iovec

// SizeOfIovec ...
const SizeOfIovec = C.sizeof_iovec

// Int ...
func Int(i int) C.int {
	return C.int(i)
}

// Malloc ...
func Malloc(n uintptr) unsafe.Pointer {
	return C.malloc(C.size_t(n))
}

// Free ...
func Free(p unsafe.Pointer) {
	C.free(p)
}

// DummyCCall ...
func DummyCCall(p unsafe.Pointer) {
	C.dummyCall(p)
}

// FillBuffersWithX ...
func FillBuffersWithX(iovec *Iovec, n int) {
	C.fillBufsWithX((*C.iovec)(iovec), C.int(n))
}
