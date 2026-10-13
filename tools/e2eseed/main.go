// Command e2eseed changes a dstask repository through the plugin's own store,
// for the end-to-end tests in e2e/. Those tests are a separate module, so
// that their libraries cannot change the versions the plugin is built with,
// and a separate module cannot import internal/store. They run this instead.
//
// Usage: go run ./tools/e2eseed <repo> <op> <arg> [<op> <arg>]...
//
// The ops run in order. "add" takes a task in dstask syntax, and "done",
// "start" and "stop" take a task ID.
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/cgardner/herdr-dstask/internal/store"
)

func main() {
	if len(os.Args) < 2 || len(os.Args)%2 != 0 {
		fail(fmt.Errorf("usage: e2eseed <repo> <op> <arg> [<op> <arg>]..."))
	}
	os.Setenv("DSTASK_CONTEXT", "")
	s := store.NewAt(os.Args[1])
	for i := 2; i < len(os.Args); i += 2 {
		op, arg := os.Args[i], os.Args[i+1]
		if op == "add" {
			fail(s.Add(arg))
			continue
		}
		id, err := strconv.Atoi(arg)
		if err != nil {
			fail(fmt.Errorf("%s needs a task id, not %q", op, arg))
		}
		switch op {
		case "done":
			fail(s.Done(id))
		case "start":
			fail(s.Start(id))
		case "stop":
			fail(s.Stop(id))
		default:
			fail(fmt.Errorf("unknown op %q", op))
		}
	}
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2eseed:", err)
		os.Exit(1)
	}
}
