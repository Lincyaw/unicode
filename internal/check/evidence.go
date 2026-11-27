package check

import "github.com/lincyaw/unicode/internal/graph"

// runtimeEvidence applies rule 37 only to actual current business executions.
// An environment error has no failed case and cannot contradict a claim.
func (c *checker) runtimeEvidence() {
	reported := map[string]bool{}
	for _, result := range c.nodes {
		if result.Layer != graph.LayerEvidence || result.Kind != graph.NodeKindResult || result.Form != "case" || c.attr(result.Id, "fresh") != "true" || c.attr(result.Id, "rejected") == "true" || c.attr(result.Id, "passed") != "false" {
			continue
		}
		checkID := c.attr(result.Id, "check")
		check, exists := c.byID[checkID]
		if !exists || check.Layer != graph.LayerDesign || check.Kind != graph.NodeKindProperty && check.Kind != graph.NodeKindFlow {
			continue
		}
		if !reported[checkID] {
			c.fail("check.runtime-counterexample", checkID, graph.StatusContradicted, check.Origin,
				"a current frozen input contradicts this check", "repair the implementation or correct the claim, then replay the recorded input")
			reported[checkID] = true
		}
		codes, subjects := result.Attrs["diagnosticCodes"], result.Attrs["diagnosticSubjects"]
		for index, code := range codes {
			if code != "lang.golang.purity-violation" || index >= len(subjects) {
				continue
			}
			subject := subjects[index]
			operation, exists := c.byID[subject]
			if !exists || operation.Layer != graph.LayerDesign || operation.Kind != graph.NodeKindOp || reported["pure:"+subject] {
				continue
			}
			c.fail("check.runtime-purity-violation", subject, graph.StatusContradicted, operation.Origin,
				"a current execution changed the state or result of this pure operation", "make the operation preserve its receiver and arguments and return the same model value on repeated calls")
			reported["pure:"+subject] = true
		}
	}
}
