# PtrGuard
[![Build Status](https://github.com/ansiwen/ptrguard/actions/workflows/go.yml/badge.svg)](https://github.com/ansiwen/ptrguard/actions)

PtrGuard is a small Go package that allows to pin objects referenced by a Go
pointer (that is pointing to memory allocated by the Go runtime) so that it will
not be touched by the garbage collector. This is done by creating a `Pinner`
object that has a `Pin()` method, which accepts a pointer of any type and pins
the referenced object, until the `Unpin()` method of the same `Pinner` is
called. A `Pinner` can be used to pin more than one object, in which case
`Unpin()` releases all the pinned objects of a `Pinner`.

Go pointers to pinned objects can either be directly stored in C memory with the
`Store()` method, or are allowed to be contained in Go memory that is passed to
C functions, which both usually violates the [pointer passing
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

With PtrGuard both is still possible. (See examples.)
