# PtrGuard
PtrGuard is a small Go package that allows to pin a Go pointer (that is pointing
to memory allocated by the Go runtime) so that the pointer will not be touched
by the garbage collector until the Release() method has been called. Therefore
the pinned Go pointer can either be directly stored in C memory with the Poke()
method or is allowed to be contained in Go memory that is passed to C functions,
which both usually violates the [pointer passing
rules](https://golang.org/cmd/cgo/#hdr-Passing_pointers). 

## Example
Let's say we want to use a C API that uses [vectored
I/O](https://en.wikipedia.org/wiki/Vectored_I/O), like the `readv()` call from
the standard C library, in order to read data into an array of buffers. Because
we want to avoid making a copy of the data, we want to read directly into Go
buffers. The pointer passing rules wouldn't allow that, because
* either we can allocate the buffer array in C memory, but then we can't store
  the pointers of the Go buffers in it. (Storing Go pointers in C memory is
  forbidden.)
* or we would allocate the buffer array in Go memory and store the Go buffers in
  it. But then we can't pass the pointer to that buffer array to a C function.
  (Passing a Go pointer that points to memory containing other Go pointers to a
  C function is forbidden.)

With PtrGuard both is still possible.

### Allocating the buffer array in C memory

```go
func ReadFileIntoBufferArray(f *os.File, bufferArray [][]byte) int {
  numberOfBuffers := len(bufferArray)

  cPtr := C.malloc(C.size_t(C.sizeof_struct_iovec * numberOfBuffers))
  defer C.free(cPtr)
  iovec := (*[math.MaxInt32]C.struct_iovec)(cPtr)[:numberOfBuffers:numberOfBuffers]

  for i := range iovec {
    bufferPtr := unsafe.Pointer(&bufferArray[i][0])
    pg := ptrguard.Pin(bufferPtr)
    defer pg.Release()
    pg.Poke(&iovec[i].iov_base)
    iovec[i].iov_len = C.size_t(len(bufferArray[i]))
  }

  n := C.readv(C.int(f.Fd()), &iovec[0], C.int(numberOfBuffers))
  return int(n)
}
```

### Allocationg the buffer array in Go memory

```go
func ReadFileIntoBufferArray(f *os.File, bufferArray [][]byte) {
  numberOfBuffers := len(bufferArray)

  type iovecT struct {
    iov_base uintptr
    iov_len  C.size_t
  }

  iovec := make([]iovecT, numberOfBuffers)

  for i := range iovec {
    bufferPtr := unsafe.Pointer(&bufferArray[i][0])
    pg := ptrguard.Pin(bufferPtr)
    defer pg.Release()
    iovec[i].iov_base = uintptr(bufferPtr)
    iovec[i].iov_len = C.size_t(len(bufferArray[i]))
  }

  n := C.readv(C.int(f.Fd()), (*C.struct_iovec)(unsafe.Pointer(&iovec[0])),
               C.int(numberOfBuffers))
  return int(n)
}
```
