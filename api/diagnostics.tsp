import "@typespec/http";

using Http;

namespace Unicode.Viewer;

/** One violation of a constraint, as `unicode check --format json` writes it (diag.Diagnostic). */
model Diagnostic {
  /** A stable code, written `<component>.<kebab-name>` */
  code: string;

  /** The identity of the node, or `kind from to` of the edge, that breaks the constraint; a file's path for syntax */
  subject: string;

  path: string;
  line: int32;
  column: int32;

  /** The cause */
  message: string;

  /** The fix */
  fix: string;
}

/** The result of checking the snapshot, as `unicode check --format json` writes it. */
model CheckReport {
  diagnostics: Diagnostic[];

  /** The status of every declared item, by identity, or by `kind from to` for an edge */
  statuses: Record<Status>;
}

/** The diagnostics of the snapshot, sorted by file, line, column and code, and every status: the header's ⚠ list. */
@route("/diagnostics")
@get
op getDiagnostics(): CheckReport | Refusals;
