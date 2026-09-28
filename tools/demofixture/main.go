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
	"path/filepath"
	"strings"
	"time"

	"github.com/naggie/dstask"

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

// done are finished tasks, so the project view has progress to show. Each
// was finished the given number of days ago.
var done = []struct {
	task string
	days int
}{
	{"+release project:lighthouse freeze the 2.0 feature list", 12},
	{"+backend project:lighthouse add retries to the job runner", 9},
	{"+backend project:lighthouse load-test the new broker", 6},
	{"+docs project:lighthouse draft the 2.0 release notes", 3},
	{"+infra project:harbor rotate the staging database password", 20},
	{"+frontend project:atlas redesign the search results page", 15},
	{"+home project:garden turn the compost", 30},
	{"+home project:garden fix the gate latch", 2},
	{"+docs project:onboarding write the first-week checklist", 45},
	{"+docs project:onboarding record the setup walkthrough", 40},
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
	// The finished tasks come after the open ones, so the open tasks keep
	// IDs 1 to 10. A resolved task gives its ID back, so each finished task
	// takes the next free ID, 11, and gives it back again.
	for _, d := range done {
		fail(s.Add(d.task))
		fail(s.Done(len(tasks) + 1))
	}
	fail(backdate(dir))

	// One active task and one paused one, so both status marks appear.
	fail(s.Start(2))
	fail(s.Start(5))
	fail(s.Stop(5))
	// Notes for the detail screenshot: prose and a checklist, as real notes
	// often have.
	fail(s.Note(2, strings.Join([]string{
		"Drain the old queue before the cut-over, then flip the feature flag.",
		"",
		"- [x] mirror traffic to the new broker",
		"- [x] compare delivery latency for a week",
		"- [ ] drain the old queue",
		"- [ ] remove the old client library",
	}, "\n")))
}

// backdate moves each finished task's resolved time into the past, so the
// project view shows different ages. The store has no call for it, because
// the plugin never needs one, so this uses the dstask library directly.
func backdate(dir string) error {
	ts, err := dstask.LoadTaskSet(dir, filepath.Join(dir, ".git", "dstask", "ids.bin"), true)
	if err != nil {
		return err
	}
	days := map[string]int{}
	for _, d := range done {
		days[dstask.ParseQuery(append([]string{dstask.CMD_ADD}, strings.Fields(d.task)...)...).Text] = d.days
	}
	for _, t := range ts.AllTasks() {
		if n, ok := days[t.Summary]; ok {
			t.Resolved = time.Now().Add(-time.Duration(n) * 24 * time.Hour)
			if err := ts.UpdateTask(t); err != nil {
				return err
			}
		}
	}
	ts.SavePendingChanges()
	return dstask.GitCommit(dir, "Backdate the finished demo tasks")
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "demofixture:", err)
		os.Exit(1)
	}
}
