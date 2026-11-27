import "@typespec/http";
import "@typespec/openapi";
import "./graph.tsp";
import "./snapshot.tsp";
import "./views.tsp";
import "./entities.tsp";
import "./queries.tsp";
import "./diagnostics.tsp";
import "./code.tsp";
import "./compare.tsp";
import "./evidence.tsp";

using Http;
using OpenAPI;

/**
 * The interface of the unicode viewer (docs/design.md §7.5). `unicode serve` answers it from the snapshot that
 * `unicode build` wrote to .unicode/graph.json, checked on load as `unicode check` checks it. It is read-only: the
 * viewer keeps its selection, tabs, expansions and navigation trail in the page address, not on the server.
 *
 * Identities are those of docs/model.md §7. They contain ".", "#", ":" and "/", so every identity travels in a query
 * parameter, never in a path segment. JSON field names are the design's, in lower camel case, as the command line's
 * JSON already writes them.
 *
 * Fields marked M6 (evidence: runs, results, coverage) and M7 (review records) are optional: the graph does not hold
 * them yet, and the server leaves them out until it does.
 */
@service(#{ title: "unicode viewer API" })
@info(#{ version: "0.1.0" })
@server("/api", "Served by unicode serve, beside the viewer's pages")
namespace Unicode.Viewer;

/** The body of every refused request, worded like a diagnostic: a stable code, the cause and the fix. */
model Problem {
  /** A stable code, written `serve.<kebab-name>` */
  code: string;

  /** The cause */
  message: string;

  /** How to make the request succeed */
  fix: string;
}

/** The request names a node, an edge or a file that the snapshot does not have (`serve.not-found`). */
@error
model NotFound {
  @statusCode _: 404;
  ...Problem;
}

/**
 * The request is malformed or does not apply: a view that does not take the input (`serve.not-applicable`), a
 * revision that git cannot read (`serve.bad-revision`), or a path outside the repository (`serve.bad-path`).
 */
@error
model BadRequest {
  @statusCode _: 400;
  ...Problem;
}
