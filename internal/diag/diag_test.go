package diag

import "pgregory.net/rapid"

//uc:generates diag.flatten-copies-fields.diagnostic
//uc:generates diag.unflatten-inverts-flatten.diagnostic
func genDiagnostic() *rapid.Generator[Diagnostic] {
	return rapid.Make[Diagnostic]()
}
