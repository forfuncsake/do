// Copyright 2017 Dave Russell. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the project LICENSE file.

// package do provides a basic mechanism to complete a group of 0 or more tasks
// after any of a set of triggers is fired. Tasks in a group are started concurrently,
// however groups of tasks can be queued to run serially, in a particular order.
package do

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
)

// Task is a function that takes no arguments and returns nothing
type Task func()

// Doer is the interface that provides all of the functionality around running tasks
//
// The Started() chan will be unblocked (closed) after any trigger declared in the When()
// call is fired
//
// The Done() chan will be unblocked (closed) when the list of tasks added via Do() calls
// is complete
//
// Call Do() any number of times (not safe for concurrent calls) to extend the list
// of tasks that the Doer will run (tasks are executed concurrently) when triggered.
//
// Call Then() any number of times (not safe for concurrent calls) to add a set of tasks
// that must wait until the previous set of tasks has completed. Note that Then() returns
// a different Doer with the current Doer's Done() chan as its trigger. Do() can be called
// on this new Doer to extend the list of tasks that are started after the previous list.
type Doer interface {
	Started() <-chan struct{}
	Done() <-chan struct{}
	Do(...Task) Doer
	Then(...Task) Doer
}

type doer struct {
	ctx      context.Context
	trigger  context.CancelFunc
	started  <-chan struct{}
	finished chan struct{}
	done     chan struct{}
	tasks    []Task
	then     *doer
}

// Started returns a channel that will be unblocked (closed) when any of the Doer's triggers fire
func (d *doer) Started() <-chan struct{} {
	return d.ctx.Done()
}

// Done returns a channel that will be unblocked (closed) when the Doer's tasks are complete.
func (d *doer) Done() <-chan struct{} {
	return d.done
}

// Do accepts a variadic set of Tasks that will be run concurrently when any trigger is fired.
// Can be run multiple times to extend the list of tasks, but is not safe to be called concurrently.
func (d *doer) Do(tasks ...Task) Doer {
	d.tasks = append(d.tasks, tasks...)
	return d
}

// Then creates and returns a new Doer that will run its tasks after the initial Doer is "Done".
// If you're holding a copy of a Doer that already has a "then dependent", the call to Then()
// will extend the list of tasks
func (d *doer) Then(tasks ...Task) Doer {
	if len(tasks) == 0 {
		return d
	}
	if d.then == nil {
		t := When(d.ctx, ChanUnblocked(d.Done()))
		d.then = t.(*doer)
	}
	d.then.Do(tasks...)

	return d.then
}

// This might need to be exported for documentation purposes
type trigger func(*doer)

// Set signals that, when received will trigger the Doer's Started() channel.
func Signal(s ...os.Signal) trigger {
	return func(d *doer) {
		c := make(chan os.Signal)
		signal.Notify(c, s...)

		go func() {
			select {
			case <-c:
				// "pull the trigger"
				d.trigger()
			case <-d.ctx.Done():
				// Bail out
			}
		}()
	}
}

// Pass a chan struct{} (perhaps some other context's Done() channel), trigger when it unblocks.
func ChanUnblocked(c <-chan struct{}) trigger {
	return func(d *doer) {
		go func() {
			select {
			case <-c:
				// "pull the trigger"
				d.trigger()
			case <-d.ctx.Done():
				// Bail out
			}
		}()
	}
}

type Mux interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

// The given handler will be attached to the given Mux (e.g. http.DefaultServeMux)
// with the provided pattern/path. Any matched request will trigger the Doer, then
// pass the request on to the provided http.HandlerFunc for processing.
// Caller is responsible to start serving on the mux.
func HTTP(mux Mux, pattern string, handler http.HandlerFunc) trigger {
	return func(d *doer) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			d.trigger()
			handler(w, r)
		})
	}
}

// When sets up a do.Doer with triggers that will cause unblocking (by closure) of the channels returned by its Started() and Done() methods.
func When(ctx context.Context, triggers ...trigger) Doer {
	triggerctx, fire := context.WithCancel(ctx)

	d := &doer{
		ctx:      ctx,
		trigger:  fire,
		started:  triggerctx.Done(),
		finished: make(chan struct{}),
		done:     make(chan struct{}),
	}

	// Apply functional option triggers, in the order received
	for _, t := range triggers {
		t(d)
	}

	go func() {
		// Any write to finish indicates that we're ready to return
		<-d.finished
		close(d.done)
	}()

	go func() {
		select {
		case <-d.started:
			// This is the meaty bit.. do all the things!
			// TODO: Should be able to stop if the parent context is canceled
			wg := new(sync.WaitGroup)
			wg.Add(len(d.tasks))
			for _, t := range d.tasks {
				go func(f Task) {
					defer wg.Done()
					if f != nil {
						f()
					}
				}(t)
			}
			wg.Wait()

			d.finish()

		case <-d.ctx.Done():
			// parent context was canceled. Go straight to "done" without any action
			d.finish()
		}
	}()

	return d
}

func (d *doer) finish() {
	select {
	case d.finished <- struct{}{}:
	default:
	}
}
