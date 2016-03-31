/***
Copyright 2014 Cisco Systems Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
Package errored implements specialized errors for golang that come with:

  * Debug and Trace modes
  * Debug emits the location the error was created, Trace emits the whole stack.
  * Error combination
  * Make two errors into one; carries the trace information for both errors with it!

Use it just like `fmt`:

	package main

	import "github.com/contiv/errored"

	func main() {
		err := errored.Errorf("a message")
		err.SetDebug(true)
		err.Error() // => "a message [ <file> <line> <line number> ]
		err2 := errored.Errorf("another message")
		combined := err.Combine(err2)
		combined.SetTrace(true)
		combined.Error() // => "a message: another message" + two stack traces
	}

*/
package errored

import (
	"fmt"
	"path"
	"runtime"
)

var (
	// AlwaysTrace will, if set globally, enable tracing on all errors.
	AlwaysTrace bool
	// AlwaysDebug will, if set globally, enable debug messages on all errors.
	AlwaysDebug bool
)

type errorStack struct {
	file string
	line int
	fun  string
}

// Error is our custom error with description, file, and line.
type Error struct {
	Code int

	desc  string
	stack [][]errorStack
	trace bool
	debug bool
}

// SetTrace enables the tracing capabilities of errored's Error struct.
//
// Please note that SetTrace automatically sets debug mode too if enabled. See SetDebug.
func (e *Error) SetTrace(trace bool) {
	e.trace = trace
	if trace {
		e.debug = trace
	}
}

// SetDebug enables the per-error caller information of errored's Error struct.
func (e *Error) SetDebug(debug bool) {
	e.debug = debug
}

// Combine combines two errors into a single one.
func (e *Error) Combine(e2 error) *Error {
	if _, ok := e2.(*Error); ok {
		return &Error{
			desc:  fmt.Sprintf("%v: %v", e.desc, e2.(*Error).desc),
			stack: append(e.stack, e2.(*Error).stack...),
		}
	}

	return &Error{
		desc:  fmt.Sprintf("%v: %v", e.desc, e2.Error()),
		stack: e.stack,
	}
}

// Error() allows *core.Error to present the `error` interface.
func (e *Error) Error() string {
	desc := e.desc
	if e.Code != 0 {
		desc = fmt.Sprintf("%d %v", e.Code, desc)
	} else {
		desc = fmt.Sprintf("%v", desc)
	}

	if e.trace || AlwaysTrace {
		ret := desc + "\n"

		for _, stack := range e.stack {
			for _, line := range stack {
				ret += fmt.Sprintf("\t%s [%s %d]\n", line.fun, line.file, line.line)
			}
		}

		return ret
	} else if (e.debug || AlwaysDebug) && len(e.stack) > 0 && len(e.stack[0]) > 0 {
		return fmt.Sprintf("%s [%s %s %d]", desc, e.stack[0][0].fun, e.stack[0][0].file, e.stack[0][0].line)
	}

	return desc
}

// Errorf returns an *Error based on the format specification provided.
func Errorf(f string, args ...interface{}) *Error {
	e := &Error{
		stack: [][]errorStack{},
		desc:  fmt.Sprintf(f, args...),
	}

	i := 1

	errors := []errorStack{}

	for {
		stack := errorStack{}
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}

		fun := runtime.FuncForPC(pc)
		if fun != nil {
			stack.fun = fun.Name()
		}

		stack.file = path.Base(file)
		stack.line = line
		errors = append(errors, stack)

		i++
	}

	e.stack = append(e.stack, errors)

	return e
}
