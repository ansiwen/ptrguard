# PtrGuard
PtrGuard is a small Go package that allows to pin a Go pointer (that is pointing
to memory allocated by the Go runtime) so that it will not be touched by the
garbage collector. This can be either done directly with the `Pin()` function,
in which case the pointer will be pinned, until the `Unpin()` method of the
returned value is called. Alternatively a `Scope()` can be created, which
provides a `Pinner` function. In this case the pointer is pinned until the scope
is left.

Pinned Go pointers can either be directly stored in C memory with the `Poke()`
method, or are allowed to be contained in Go memory that is passed to C
functions, which both usually violates the [pointer passing
rules](https://golang.org/cmd/cgo/#hdr-Passing_pointers). In the second case you
might need the `NoCheck()` helper function to call the C function in a context,
where the cgocheck debug feature is disabled. This is necessary because PtrGuard
doesn't have any possibility to tell cgocheck, that certain pointers are pinned.

## Example
Let's say we want to use a C API that uses [vectored
I/O](https://en.wikipedia.org/wiki/Vectored_I/O), like the
[`readv()`](https://pubs.opengroup.org/onlinepubs/000095399/functions/readv.html)
POSIX system call, in order to read data into an array of buffers. Because we
want to avoid making a copy of the data, we want to read directly into Go
buffers. The pointer passing rules wouldn't allow that, because
* either we can allocate the buffer array in C memory, but then we can't store
  the pointers of the Go buffers in it. (Storing Go pointers in C memory is
  forbidden.)
* or we would allocate the buffer array in Go memory and store the Go buffers in
  it. But then we can't pass the pointer to that buffer array to a C function.
  (Passing a Go pointer that points to memory containing other Go pointers to a
  C function is forbidden.)

With PtrGuard both is still possible:

### Allocating the buffer array in C memory

#### direct API

```go
func ReadFileIntoBufferArray(f *os.File, bufferArray [][]byte) int {
	numberOfBuffers := len(bufferArray)

	cPtr := C.malloc(C.size_t(C.sizeof_struct_iovec * numberOfBuffers))
	defer C.free(cPtr)
	iovec := (*[math.MaxInt32]C.struct_iovec)(cPtr)[:numberOfBuffers:numberOfBuffers]

	var n C.ssize_t
	for i := range iovec {
		bufferPtr := unsafe.Pointer(&bufferArray[i][0])
		defer ptrguard.Pin(bufferPtr).Poke(&iovec[i].iov_base).Unpin()
		iovec[i].iov_len = C.size_t(len(bufferArray[i]))
	}
	n = C.readv(C.int(f.Fd()), &iovec[0], C.int(numberOfBuffers))
	return int(n)
}
```

#### scope API

```go
func ReadFileIntoBufferArray(f *os.File, bufferArray [][]byte) int {
	numberOfBuffers := len(bufferArray)

	cPtr := C.malloc(C.size_t(C.sizeof_struct_iovec * numberOfBuffers))
	defer C.free(cPtr)
	iovec := (*[math.MaxInt32]C.struct_iovec)(cPtr)[:numberOfBuffers:numberOfBuffers]

	var n C.ssize_t
	ptrguard.Scope(func(pin ptrguard.Pinner) {
		for i := range iovec {
			bufferPtr := unsafe.Pointer(&bufferArray[i][0])
			pin(bufferPtr).Poke(&iovec[i].iov_base)
			iovec[i].iov_len = C.size_t(len(bufferArray[i]))
		}
		n = C.readv(C.int(f.Fd()), &iovec[0], C.int(numberOfBuffers))
	})
	return int(n)
}
```

### Allocating the buffer array in Go memory

#### direct API

```go
func ReadFileIntoBufferArray(f *os.File, bufferArray [][]byte) int {
	numberOfBuffers := len(bufferArray)

	iovec := make([]C.struct_iovec, numberOfBuffers)

	var n C.ssize_t
	for i := range iovec {
		bufferPtr := unsafe.Pointer(&bufferArray[i][0])
		defer ptrguard.Pin(bufferPtr).Unpin()
		iovec[i].iov_base = bufferPtr
		iovec[i].iov_len = C.size_t(len(bufferArray[i]))
	}
	ptrguard.NoCheck(func() {
		n = C.readv(C.int(f.Fd()), &iovec[0], C.int(numberOfBuffers))
	})
	return int(n)
}
```

#### scope API

```go
func ReadFileIntoBufferArray(f *os.File, bufferArray [][]byte) int {
	numberOfBuffers := len(bufferArray)

	iovec := make([]C.struct_iovec, numberOfBuffers)

	var n C.ssize_t
	ptrguard.Scope(func(pin ptrguard.Pinner) {
		for i := range iovec {
			bufferPtr := unsafe.Pointer(&bufferArray[i][0])
			pin(bufferPtr)
			iovec[i].iov_base = bufferPtr
			iovec[i].iov_len = C.size_t(len(bufferArray[i]))
		}
		pg.NoCheck(func() {
			n = C.readv(C.int(f.Fd()), &iovec[0], C.int(numberOfBuffers))
		})
	})
	return int(n)
}
```
