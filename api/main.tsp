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
import "./reviews.tsp";

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
 * Evidence and review fields are optional until a measurement or a matching review exists; unmeasured data is
 * absent rather than reported as a successful or complete check.
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

/** The request names a node, an edge, a file or an endpoint that the snapshot or the interface does not have (`serve.not-found`). */
@error
model NotFound {
  @statusCode _: 404;
  ...Problem;
}

/**
 * The request is malformed or does not apply: a view that does not take the input (`serve.not-applicable`), a
 * revision that git cannot read (`serve.bad-revision`), a path outside the repository (`serve.bad-path`), or a
 * parameter that is missing, not of its type, or not read by the view it is given to (`serve.bad-parameter`).
 */
@error
model BadRequest {
  @statusCode _: 400;
  ...Problem;
}

/** The request is not a GET or a HEAD: the interface is read-only (`serve.bad-parameter`). */
@error
model MethodNotAllowed {
  @statusCode _: 405;
  ...Problem;
}

/** The server failed to answer: the graph of a base revision cannot be built, or the server itself failed (`serve.internal-error`). */
@error
model ServerError {
  @statusCode _: 500;
  ...Problem;
}

/**
 * The snapshot file stopped being readable while the server runs (`serve.unreadable-snapshot`); once it is readable
 * again, the next request is answered.
 */
@error
model Unavailable {
  @statusCode _: 503;
  ...Problem;
}

/** The refusals that any request can get, whatever it asks. */
alias Refusals = MethodNotAllowed | ServerError | Unavailable;
