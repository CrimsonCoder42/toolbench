//go:build race

package recycle_test

// raceEnabled is true under -race, where sync.Pool drops a random quarter of Puts on purpose, so a Get right
// after a Put may call newFn.
const raceEnabled = true
