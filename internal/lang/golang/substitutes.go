package golang

import (
	"go/types"
	"strconv"

	"github.com/lincyaw/unicode/internal/graph"
)

func installerMatches(installer, client *types.Signature) bool {
	if installer == nil || client == nil || installer.Recv() != nil || installer.Variadic() || installer.Params().Len() != 1 || installer.Results().Len() != 1 {
		return false
	}
	replacement, ok := types.Unalias(installer.Params().At(0).Type()).Underlying().(*types.Signature)
	if !ok {
		return false
	}
	cleanup, ok := types.Unalias(installer.Results().At(0).Type()).Underlying().(*types.Signature)
	if !ok || cleanup.Params().Len() != 0 || cleanup.Results().Len() != 0 || cleanup.Variadic() {
		return false
	}
	identity := nativeIdentity{packages: map[string]*types.Package{}, named: map[string][]nativeNamed{}}
	return types.Identical(identity.typ(replacement), identity.typ(client))
}

func (x *extraction) substituteSignatures() {
	functions := map[string]*types.Func{}
	for _, d := range x.declared {
		if d.fn != nil {
			functions[d.id] = d.fn
		}
	}
	for _, e := range x.edges {
		if e.Kind != graph.EdgeKindSubstitutes {
			continue
		}
		var clients []*types.Func
		for _, binding := range x.edges {
			if binding.Kind == graph.EdgeKindRealizes && binding.To == e.To && functions[binding.From] != nil {
				clients = append(clients, functions[binding.From])
			}
		}
		valid := false
		if installer := functions[e.From]; installer != nil && len(clients) == 1 {
			valid = installerMatches(installer.Signature(), clients[0].Signature())
		}
		e.Attrs = map[string][]string{"nativeSignature": {strconv.FormatBool(valid)}}
	}
}
