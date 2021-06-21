package test_c_helper

/*
#include <stdlib.h>

void dummyCall(void* p) {}

*/
import "C"
import "unsafe"

func Malloc(n uintptr) unsafe.Pointer {
	return C.malloc(C.size_t(n))
}

func Free(p unsafe.Pointer) {
	C.free(p)
}

func DummyCCall(p unsafe.Pointer) {
	C.dummyCall(p)
}
