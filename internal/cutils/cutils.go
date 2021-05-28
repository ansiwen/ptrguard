package cutils

/*
#include <stdlib.h>
*/
import "C"
import "unsafe"

func Malloc(n uintptr) unsafe.Pointer {
	return C.malloc(C.size_t(n))
}

func Free(p unsafe.Pointer) {
	C.free(p)
}
