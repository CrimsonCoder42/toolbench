package recycle_test

import (
	"bytes"
	"fmt"

	"github.com/CrimsonCoder42/toolbench/recycle"
)

func ExamplePool() {
	bufs := recycle.New(
		func() *bytes.Buffer { return new(bytes.Buffer) },
		recycle.WithReset(func(b *bytes.Buffer) *bytes.Buffer { b.Reset(); return b }),
	)

	b := bufs.Get() // a *bytes.Buffer, no type assertion
	b.WriteString("hello")
	fmt.Println(b.String())
	bufs.Put(b) // reset, then pooled

	fmt.Println(bufs.Get().Len()) // reused or new: empty either way
	// Output:
	// hello
	// 0
}

func ExampleNew() {
	var zero recycle.Pool[*bytes.Buffer] // the zero value is ready to use
	fmt.Println(zero.Get() == nil)

	ints := recycle.New[int](nil) // a nil newFn: Get on an empty pool returns the zero T, no panic
	fmt.Println(ints.Get())
	// Output:
	// true
	// 0
}

func ExampleOption() {
	opts := []recycle.Option[*[]byte]{
		recycle.WithReset(func(b *[]byte) *[]byte { *b = (*b)[:0]; return b }),
		nil, // ignored
	}
	p := recycle.New(func() *[]byte { b := make([]byte, 0, 16); return &b }, opts...)
	b := p.Get()
	*b = append(*b, "abc"...)
	p.Put(b)
	fmt.Println(len(*p.Get()))
	// Output: 0
}

func ExampleWithReset() {
	p := recycle.New(
		func() *[]int { s := make([]int, 0, 8); return &s },
		recycle.WithReset(func(s *[]int) *[]int { *s = (*s)[:0]; return s }),
	)
	s := p.Get()
	*s = append(*s, 1, 2, 3)
	p.Put(s) // length cut to 0, capacity kept
	got := p.Get()
	fmt.Println(len(*got), cap(*got))
	// Output: 0 8
}

func ExampleWithKeep() {
	const maxCap = 1 << 10
	p := recycle.New(
		func() *bytes.Buffer { return bytes.NewBuffer(make([]byte, 0, 64)) },
		recycle.WithKeep(func(b *bytes.Buffer) bool { return b.Cap() <= maxCap }),
	)
	big := p.Get()
	big.Grow(1 << 20) // now far too large to keep
	p.Put(big)        // dropped, not pooled

	fmt.Println(p.Get().Cap() <= maxCap)
	// Output: true
}
