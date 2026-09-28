// Command demofixture writes an invented dstask repository for screenshots.
//
// Documentation images must never show a real task list: a real list names an
// employer, colleagues, customers and private work, and a README is public.
// This makes the repository through the plugin's own store, so the picture is
// the real rendering path with nothing real in it.
//
// Usage: go run ./tools/demofixture <dir>
package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/cgardner/herdr-dstask/internal/store"
)

// tasks are added in order, so each gets the next ID from 1.
var tasks = []string{
	"P0 +release project:lighthouse due:tomorrow cut the 2.0 release candidate",
	"P1 +backend project:lighthouse migrate the job queue to the new broker",
	"P1 +infra project:harbor due:yesterday renew the staging TLS certificates",
	"P1 +review project:lighthouse review the retry-policy pull request",
	"P2 +docs project:lighthouse write the upgrade guide for 2.0",
	"P2 +frontend project:atlas add keyboard shortcuts to the search page",
	"P2 +backend project:atlas profile the slow export endpoint",
	"P2 +home project:garden order seeds for the spring beds",
	"P3 +reading read the paper on consistent hashing",
	"P3 +someday project:garden build a cold frame",
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: demofixture <dir>")
		os.Exit(2)
	}
	dir := os.Args[1]
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "demo@example.com"},
		{"config", "user.name", "demo"},
		{"config", "commit.gpgsign", "false"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			fail(fmt.Errorf("git %v: %v: %s", args, err, out))
		}
	}
	os.Setenv("DSTASK_CONTEXT", "")
	s := store.NewAt(dir)
	for _, t := range tasks {
		fail(s.Add(t))
	}
	// One active task and one paused one, so both status marks appear.
	fail(s.Start(2))
	fail(s.Start(5))
	fail(s.Stop(5))
	fail(s.Note(2, "Drain the old queue before the cut-over, then flip the feature flag."))
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "demofixture:", err)
		os.Exit(1)
	}
}
