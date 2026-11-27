package syntax

import "pgregory.net/rapid"

//uc:generates syntax.rejects-broken-text.path
//uc:generates syntax.contract-syntax-rejects-invalid-claims.path
//uc:generates syntax.observation-syntax-rejects-malformed-paths.path
func genBrokenPath(broken Broken) *rapid.Generator[string] {
	return rapid.Just(samplePaths[broken.Kind])
}

//uc:generates syntax.rejects-broken-text.src
//uc:generates syntax.contract-syntax-rejects-invalid-claims.src
//uc:generates syntax.observation-syntax-rejects-malformed-paths.src
func genBrokenSource(broken Broken, path string) *rapid.Generator[[]byte] {
	return rapid.Just([]byte(broken.Text))
}
