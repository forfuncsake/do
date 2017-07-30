// Copyright 2017 Dave Russell. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the project LICENSE file.

// This example show use of `package do`, chaining `Then()` calls
// to order our numeric output
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

	// Proof that a/b/c are unordered (started concurrently, then at the mercy of the scheduler)
	// but separate Then() calls are always ordered
	d := do.When(context.Background(), do.ChanUnblocked(ctx.Done())).Do(
		push("a"),
		push("b"),
		push("c"),
	).Then(
		push("1"),
	).Then(
		push("2"),
	).Then(
		push("3"),
	)

	go func() {
		<-d.Done()
		close(chars)
	}()

	cancel()
	for char := range chars {
		fmt.Println("got: ", char)
	}
}
