import "@typespec/http";

using Http;

namespace Unicode.Viewer;

/** Lines of a file of the repository, as its working tree holds them. */
model CodeExcerpt {
  /** Relative to the repository root */
  path: string;

  /** The first line, from 1 */
  from: int32;

  /** The last line, included */
  to: int32;

  /** The lines, each ending in a newline */
  text: string;

  /** The language of a code file, as the system file names it (go, python); absent for a design or corpus file */
  language?: string;
}

/**
 * Lines of a file: the whole file when no range is given. The path must be a design or corpus file, or a code file of
 * the snapshot; any other path is refused, so the server reads no file outside what the graph describes.
 */
@route("/code")
@get
op getCode(@query path: string, @query from?: int32, @query to?: int32): CodeExcerpt | NotFound | BadRequest | Refusals;
