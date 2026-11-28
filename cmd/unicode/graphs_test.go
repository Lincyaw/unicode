package main

import "pgregory.net/rapid"

// genAbsentRoot draws a relative path of absent- and one to twelve lowercase
// letters, which the test's working directory does not hold.
//
//uc:generates cli.commit-graphs-have-one-system.root
func genAbsentRoot() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		return "absent-" + rapid.StringMatching(`[a-z]{1,12}`).Draw(t, "name")
	})
}
