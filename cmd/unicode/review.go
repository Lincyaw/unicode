package main

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/graph"
	"github.com/lincyaw/unicode/internal/repo"
	"github.com/lincyaw/unicode/internal/review"
)

func reviewCommand(g *globalOptions, stdout, stderr io.Writer) *cobra.Command {
	command := &cobra.Command{Use: "review", Short: "Inspect pending content and record immutable reviews"}
	command.AddCommand(reviewListCommand(g, stdout, stderr), reviewRecordCommand(g, stdout, stderr))
	return command
}

func reviewListCommand(g *globalOptions, stdout, stderr io.Writer) *cobra.Command {
	var base string
	pending := true
	command := &cobra.Command{
		Use: "list [id...]", Short: "List pending reviews, including deletions when a base is given",
		RunE: func(command *cobra.Command, ids []string) error {
			if command.Flags().Changed("base") && base == "" {
				return usageError("--base must name a revision")
			}
			root, head, baseline, diagnostics, err := reviewGraphs(g.root, base)
			if err != nil {
				return err
			}
			if len(diagnostics) > 0 {
				return writeReport(g.format, review.Evaluation{Items: []review.Item{}, Diagnostics: diagnostics}, nil, diagnostics, stdout, stderr)
			}
			for _, id := range ids {
				if head.Node(id) == nil && (baseline == nil || baseline.Node(id) == nil) {
					return requireNodes(&head, id)
				}
			}
			loaded := review.Read(root)
			evaluated := review.Evaluate(&head, baseline, loaded.Records)
			evaluated.Diagnostics = append(evaluated.Diagnostics, loaded.Diagnostics...)
			evaluated.Items = slices.DeleteFunc(evaluated.Items, func(item review.Item) bool {
				return pending && item.State != "pending" || len(ids) > 0 && !slices.Contains(ids, item.Target.Node) && !slices.Contains(ids, item.Target.From) && !slices.Contains(ids, item.Target.To)
			})
			text := []byte{}
			for _, item := range evaluated.Items {
				text = fmt.Appendf(text, "%s %s %s\n", reviewTargetText(item.Target), item.State, item.Digest)
			}
			sortDiagnostics(evaluated.Diagnostics)
			return writeReport(g.format, evaluated, text, evaluated.Diagnostics, stdout, stderr)
		},
	}
	command.Flags().StringVar(&base, "base", "", "compare with this revision, including removed content")
	command.Flags().BoolVar(&pending, "pending", true, "list only pending content; set false to include reviewed content")
	return command
}

func reviewRecordCommand(g *globalOptions, stdout, stderr io.Writer) *cobra.Command {
	var base string
	var request review.Request
	command := &cobra.Command{
		Use: "record [id]", Short: "Record a review of the current content of one node or edge",
		Long: "record writes an immutable review of the current content of one node or edge to .unicode/reviews.\n" +
			"The viewer shows it once unicode build compiles it into the snapshot.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, ids []string) error {
			if len(ids) > 0 {
				request.Target.Node = ids[0]
			}
			edge := request.Target.EdgeKind != "" || request.Target.From != "" || request.Target.To != ""
			if len(ids) == 0 && !edge || len(ids) > 0 && edge {
				return usageError("give exactly one node identity or --edge, --from and --to")
			}
			if edge && (request.Target.EdgeKind == "" || request.Target.From == "" || request.Target.To == "") {
				return usageError("an edge review requires --edge, --from and --to")
			}
			if request.Target.Removed && base == "" {
				return usageError("--removed requires --base")
			}
			if command.Flags().Changed("base") && base == "" {
				return usageError("--base must name a revision")
			}
			if strings.TrimSpace(request.Reviewer) == "" {
				return usageError("--reviewer must name the reviewer")
			}
			if request.Decision != "approved" && request.Decision != "changes-requested" {
				return usageError("--decision must be approved or changes-requested")
			}
			root, head, baseline, diagnostics, err := reviewGraphs(g.root, base)
			if err != nil {
				return err
			}
			written := review.Written{Diagnostics: diagnostics}
			if len(diagnostics) == 0 {
				metadata, err := repo.ReadHead(root, filepath.Join(root, ".unicode"))
				if err != nil {
					return repoError(err)
				}
				request.Dirty = metadata.Dirty
				if metadata.Commit != nil {
					request.Commit = *metadata.Commit
				}
				written = review.Write(root, &head, baseline, request)
			}
			text := []byte{}
			if written.Record != nil {
				text = fmt.Appendf(text, "%s %s %s\n", reviewTargetText(written.Record.Target), written.Record.Decision, written.Record.Id)
			}
			sortDiagnostics(written.Diagnostics)
			return writeReport(g.format, written, text, written.Diagnostics, stdout, stderr)
		},
	}
	flags := command.Flags()
	flags.StringVar(&base, "base", "", "compare with this revision; required for removed content")
	flags.StringVar(&request.Target.EdgeKind, "edge", "", "kind of the edge to review")
	flags.StringVar(&request.Target.From, "from", "", "source identity of the edge to review")
	flags.StringVar(&request.Target.To, "to", "", "target identity of the edge to review")
	flags.BoolVar(&request.Target.Removed, "removed", false, "review a deletion relative to --base")
	flags.StringVar(&request.Reviewer, "reviewer", "", "identity of the reviewer")
	flags.StringVar(&request.Decision, "decision", "", "approved or changes-requested")
	flags.StringVar(&request.Comment, "comment", "", "review comment")
	flags.StringVar(&request.ExpectedDigest, "expect-digest", "", "refuse content whose current fingerprint differs")
	return command
}

func reviewGraphs(rootOption, base string) (string, graph.Graph, *graph.Graph, []diag.Diagnostic, error) {
	root, err := findRoot(rootOption)
	if err != nil {
		return "", graph.Graph{}, nil, nil, err
	}
	head, diagnostics, err := buildInputGraph(root)
	if err != nil || base == "" {
		return root, head, nil, diagnostics, err
	}
	baseline, problems, err := sinceRevisionGraph(root, base)
	if err != nil {
		return root, head, nil, diagnostics, repoError(err)
	}
	return root, head, &baseline, append(diagnostics, problems...), nil
}

func reviewTargetText(target review.Target) string {
	text := target.Node
	if text == "" {
		text = target.EdgeKind + " " + target.From + " " + target.To
	}
	if target.Removed {
		text += " (removed)"
	}
	return text
}
