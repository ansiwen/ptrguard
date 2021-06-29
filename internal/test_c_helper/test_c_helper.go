package test_c_helper //nolint

/*
#include <stdlib.h>

void dummyCall(void* p) {}

*/
import "C"
import (
	"unsafe"
)

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
