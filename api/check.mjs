// check.mjs confirms that openapi.yaml is exactly what TypeSpec emits from the
// committed sources: it emits into a temporary directory and compares the bytes.
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const out = mkdtempSync(join(tmpdir(), "unicode-api-"));
try {
  execFileSync("tsp", ["compile", ".", "--warn-as-error", "--option", `@typespec/openapi3.emitter-output-dir=${out}`], {
    stdio: ["ignore", "ignore", "inherit"],
  });
  const emitted = readFileSync(join(out, "openapi.yaml"));
  if (!emitted.equals(readFileSync("openapi.yaml"))) {
    console.error(
      "api/openapi.yaml differs from what api/*.tsp emits; it is generated, so do not edit it: change the .tsp sources and run make api",
    );
    process.exitCode = 1;
  }
} finally {
  rmSync(out, { recursive: true, force: true });
}
