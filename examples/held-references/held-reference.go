// Copyright 2017 Dave Russell. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the project LICENSE file.

// This example application demonstrates which Doer is returned
// when making calls to `Do()` and `Then()`
package main

import (
	"context"
	"fmt"

	"github.com/forfuncsake/do"
)

func main() {
	chars := make(chan string, 10)

	push := func(i string) func() {
		return func() {
			chars <- i
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	a := do.When(context.Background(), do.ChanUnblocked(ctx.Done())).Do(
		push("a"),
	)

	b := a.Then(
		push("b"),
	)

	_ = a.Then(
		push("bb"),
	)

	c := b.Then(
		push("c"),
	)

	a = a.Do(push("aa"))
	b = b.Do(push("bbb"))
	c = c.Do(push("cc"))

	d := a.Do(push("aaa")).Then(push("bbbb")).Then(push("ccc"))

	go func() {
		<-d.Done()
		close(chars)
	}()

	cancel()
	for char := range chars {
		fmt.Println("got: ", char)
	}
}
