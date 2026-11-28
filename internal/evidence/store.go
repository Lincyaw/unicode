package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/renameio/v2/maybe"

	"github.com/lincyaw/unicode/internal/diag"
	"github.com/lincyaw/unicode/internal/lang"
	"github.com/lincyaw/unicode/monitor"
)

// Loaded retains all recorded runs, including stale and legacy history.
//
//uc:realizes evidence.Loaded
type Loaded struct {
	Reports     []lang.RunReport  `json:"reports"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

func hexDigest(text string, size int) bool {
	if len(text) != size {
		return false
	}
	for _, char := range text {
		if char < '0' || char > '9' && char < 'a' || char > 'f' {
			return false
		}
	}
	return true
}
func reportPath(id string) string { return ".unicode/runs/" + id + ".json" }

func storeDiagnostic(code, path, cause, fix string) diag.Diagnostic {
	d := diagnostic(code, path, cause, fix)
	d.Position.Path = filepath.ToSlash(path)
	return d
}

func store(root string, create bool) (string, []diag.Diagnostic) {
	if strings.TrimSpace(root) == "" {
		return "", []diag.Diagnostic{storeDiagnostic("evidence.unreadable-report", "design/system.uc", "the repository root is empty", "give the repository root containing the run history")}
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", []diag.Diagnostic{storeDiagnostic("evidence.unreadable-report", "design/system.uc", err.Error(), "give an existing repository directory")}
	}
	paths := []string{absolute, filepath.Join(absolute, ".unicode"), filepath.Join(absolute, ".unicode", "runs")}
	positions := []string{"design/system.uc", ".unicode", ".unicode/runs"}
	for index, path := range paths {
		info, problem := os.Lstat(path)
		if errors.Is(problem, fs.ErrNotExist) && index > 0 {
			if !create {
				return "", nil
			}
			if problem = os.Mkdir(path, 0o755); problem == nil || errors.Is(problem, fs.ErrExist) {
				info, problem = os.Lstat(path)
			}
		}
		if problem != nil {
			code := "evidence.unreadable-report"
			if create {
				code = "evidence.write-failed"
			}
			return "", []diag.Diagnostic{storeDiagnostic(code, positions[index], problem.Error(), "restore the run-store directory and its access permissions")}
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", []diag.Diagnostic{storeDiagnostic("evidence.symlink", positions[index], "the run-store path is a symbolic link", "replace it with a regular directory inside the repository")}
		}
		if !info.IsDir() {
			return "", []diag.Diagnostic{storeDiagnostic("evidence.unreadable-report", positions[index], "the run-store path is not a directory", "restore the local run-store directory")}
		}
	}
	return paths[2], nil
}

// canonicalTime tells whether text is a canonical UTC RFC3339Nano time.
func canonicalTime(text string) bool {
	at, err := time.Parse(time.RFC3339Nano, text)
	return err == nil && at.UTC().Format(time.RFC3339Nano) == text
}

func normalizeReport(report lang.RunReport, legacy bool) (lang.RunReport, error) {
	old := report.Version == 0 && report.Snapshot == "" && legacy
	runtime := report.Venue == lang.VenueRuntime
	switch {
	case old:
	case report.Venue != "" && report.Venue != lang.VenueTest && !runtime:
		return report, fmt.Errorf("the report venue is neither test nor runtime")
	case runtime:
		// A monitored program writes no snapshot or mode; build gives a clean
		// commit's record the execution-input digest of that commit.
		if report.Version != 1 || report.Mode != "" || report.Snapshot != "" && !hexDigest(report.Snapshot, 64) {
			return report, fmt.Errorf("the runtime record needs version 1, no mode, and no snapshot or a complete one")
		}
		if !canonicalTime(report.At) || !canonicalTime(report.From) {
			return report, fmt.Errorf("the runtime record's from and at must be canonical UTC RFC3339Nano")
		}
	default:
		if report.Version != 1 || !hexDigest(report.Snapshot, 64) || report.From != "" {
			return report, fmt.Errorf("the report needs version 1 and a complete execution-input digest")
		}
		if !canonicalTime(report.At) {
			return report, fmt.Errorf("the report time must be canonical UTC RFC3339Nano")
		}
		if report.Mode != "replay" && report.Mode != "explore" && report.Mode != "fuzz" {
			return report, fmt.Errorf("the report mode is not replay, explore or fuzz")
		}
	}
	if report.Commit != "" && !hexDigest(report.Commit, 40) && !hexDigest(report.Commit, 64) {
		return report, fmt.Errorf("the report commit must be empty or a complete Git commit hash")
	}
	if len(report.Results) == 0 && len(report.Diagnostics) == 0 {
		return report, fmt.Errorf("the report has neither execution results nor a tool error")
	}
	report.Results = slices.Clone(report.Results)
	report.Diagnostics = append([]diag.Diagnostic{}, report.Diagnostics...)
	seen := map[string]bool{}
	for index, result := range report.Results {
		if strings.TrimSpace(result.Id) == "" || seen[result.Id] {
			return report, fmt.Errorf("a check result identity is empty or repeated")
		}
		seen[result.Id] = true
		if result.Accepted < 0 || result.Rejected < 0 || result.CodeTotal < 0 || result.BranchTotal < 0 || result.ErrorTotal < 0 {
			return report, fmt.Errorf("execution and coverage counts cannot be negative")
		}
		result.Observations = append([]string{}, result.Observations...)
		result.Differences = append([]string{}, result.Differences...)
		result.Diagnostics = append([]diag.Diagnostic{}, result.Diagnostics...)
		result.Cases = slices.Clone(result.Cases)
		if !runtime && (len(result.Clauses) > 0 || result.P95 != 0 || result.Traces != 0) {
			return report, fmt.Errorf("only a runtime record counts judged clauses, percentiles and traces")
		}
		if result.P95 < 0 || result.Traces < 0 {
			return report, fmt.Errorf("execution and coverage counts cannot be negative")
		}
		for _, outside := range result.OutsideDomain {
			if outside.Clause == "" || outside.Calls < 1 {
				return report, fmt.Errorf("a count of calls outside the domain needs its clause and a positive count")
			}
		}
		for _, clause := range result.Clauses {
			if clause.Clause == "" || clause.Judged < 1 {
				return report, fmt.Errorf("a count of judged clauses needs its clause and a positive count")
			}
		}
		for at, candidate := range result.Cases {
			copied, err := cloneCase(candidate, runtime)
			if err != nil {
				return report, err
			}
			if copied.Check != result.Id {
				return report, fmt.Errorf("a case result belongs to a different check")
			}
			result.Cases[at] = copied
		}
		if result.Cases == nil {
			result.Cases = []lang.CaseResult{}
		}
		report.Results[index] = result
	}
	if report.Results == nil {
		report.Results = []lang.RunResult{}
	}
	all := slices.Clone(report.Diagnostics)
	for _, result := range report.Results {
		all = append(all, result.Diagnostics...)
		for _, candidate := range result.Cases {
			all = append(all, candidate.Diagnostics...)
		}
	}
	for _, problem := range all {
		if problem.Code == "" || problem.Position.Path == "" || problem.Position.Line < 1 || problem.Position.Column < 1 || problem.Message == "" || problem.Fix == "" {
			return report, fmt.Errorf("the report contains an incomplete diagnostic")
		}
	}
	return report, nil
}

func uniqueJSON(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delimiter == '{' {
		keys := map[string]bool{}
		for decoder.More() {
			key, problem := decoder.Token()
			if problem != nil {
				return problem
			}
			name := key.(string)
			if keys[name] {
				return fmt.Errorf("the JSON repeats field %s", name)
			}
			keys[name] = true
			if problem = uniqueJSON(decoder); problem != nil {
				return problem
			}
		}
	} else {
		for decoder.More() {
			if err := uniqueJSON(decoder); err != nil {
				return err
			}
		}
	}
	_, err = decoder.Token()
	return err
}

func diagnosticFields(value any) error {
	switch value := value.(type) {
	case map[string]any:
		if _, isCode := value["code"].(string); isCode {
			keys := []string{"code", "subject", "path", "line", "column", "message", "fix"}
			if len(value) != len(keys) {
				return fmt.Errorf("a diagnostic has missing or unknown fields")
			}
			for _, key := range keys {
				if field, ok := value[key]; !ok || field == nil {
					return fmt.Errorf("diagnostic field %s is missing or null", key)
				}
			}
		}
		for _, child := range value {
			if err := diagnosticFields(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range value {
			if err := diagnosticFields(child); err != nil {
				return err
			}
		}
	}
	return nil
}

// jsonKeys are the JSON keys of a struct type's fields: the runtime record's
// keys are those of the monitor's Record, which writes it.
func jsonKeys(t reflect.Type) []string {
	var keys []string
	for field := range t.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		keys = append(keys, name)
	}
	return keys
}

func decodeReport(data []byte, legacy bool) (lang.RunReport, error) {
	var report lang.RunReport
	if !utf8.Valid(data) {
		return report, fmt.Errorf("the report JSON is not UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return report, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return report, fmt.Errorf("more data follows the report JSON")
	}
	if err := uniqueJSON(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return report, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return report, err
	}
	if report.Version == 1 {
		keys := []string{"results", "diagnostics", "version", "snapshot", "commit", "dirty", "at", "mode"}
		switch report.Venue {
		case lang.VenueTest:
			keys = append(keys, "venue")
		case lang.VenueRuntime:
			keys = jsonKeys(reflect.TypeFor[monitor.Record]())
		}
		if len(fields) != len(keys) {
			return report, fmt.Errorf("the version 1 report has missing fields")
		}
		for _, key := range keys {
			if raw, ok := fields[key]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return report, fmt.Errorf("report field %s is missing or null", key)
			}
		}
	}
	var tree any
	if err := json.Unmarshal(data, &tree); err != nil {
		return report, err
	}
	if err := diagnosticFields(tree); err != nil {
		return report, err
	}
	return normalizeReport(report, legacy)
}

// Read reads immutable run history and the latest result file, deduplicating
// their contents. Legacy results stay available but never become current proof.
//
//uc:realizes evidence.read
func Read(root string) Loaded {
	out := Loaded{Reports: []lang.RunReport{}, Diagnostics: []diag.Diagnostic{}}
	directory, problems := store(root, false)
	if len(problems) > 0 {
		out.Diagnostics = problems
		return out
	}
	type entry struct {
		path, position, id string
		legacy             bool
	}
	files := []entry{}
	if directory != "" {
		listed, err := os.ReadDir(directory)
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, storeDiagnostic("evidence.unreadable-report", ".unicode/runs", err.Error(), "restore read access to the run history"))
			return out
		}
		for _, item := range listed {
			id, _ := strings.CutSuffix(item.Name(), ".json")
			files = append(files, entry{filepath.Join(directory, item.Name()), ".unicode/runs/" + item.Name(), id, false})
		}
	}
	files = append(files, entry{filepath.Join(root, ".unicode", "results.json"), ".unicode/results.json", "", true})
	seen := map[string]bool{}
	for _, file := range files {
		info, err := os.Lstat(file.path)
		if errors.Is(err, fs.ErrNotExist) && file.legacy {
			continue
		}
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, storeDiagnostic("evidence.unreadable-report", file.position, err.Error(), "restore read access to this report"))
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			out.Diagnostics = append(out.Diagnostics, storeDiagnostic("evidence.symlink", file.position, "the report is a symbolic link", "restore its regular immutable JSON file"))
			continue
		}
		if !info.Mode().IsRegular() || !file.legacy && (!hexDigest(file.id, 64) || filepath.Ext(file.path) != ".json") {
			out.Diagnostics = append(out.Diagnostics, storeDiagnostic("evidence.invalid-report", file.position, "the run-store entry has an invalid file type or name", "restore a regular lowercase-digest.json report"))
			continue
		}
		data, err := os.ReadFile(file.path)
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, storeDiagnostic("evidence.unreadable-report", file.position, err.Error(), "restore read access to this report"))
			continue
		}
		report, err := decodeReport(data, file.legacy)
		if err == nil && !file.legacy && hash(report) != file.id && (report.Venue != lang.VenueRuntime || rawHash(data) != file.id) {
			err = fmt.Errorf("the immutable filename does not match the canonical report content")
		}
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, storeDiagnostic("evidence.invalid-report", file.position, err.Error(), "restore the complete immutable report with its matching filename"))
			continue
		}
		id := hash(report)
		if !seen[id] {
			out.Reports = append(out.Reports, report)
			seen[id] = true
		}
	}
	slices.SortFunc(out.Reports, func(a, b lang.RunReport) int {
		first, _ := time.Parse(time.RFC3339Nano, a.At)
		second, _ := time.Parse(time.RFC3339Nano, b.At)
		if order := first.Compare(second); order != 0 {
			return order
		}
		return strings.Compare(hash(a), hash(b))
	})
	return out
}

func publish(directory, name string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(directory), ".run-pending-")
	if err != nil {
		return err
	}
	path := temp.Name()
	// Staging files lie outside runs and cannot masquerade as committed history.
	defer func() { _ = os.Remove(path) }()
	if _, err = temp.Write(data); err == nil {
		err = temp.Sync()
	}
	closed := temp.Close()
	if err == nil {
		err = closed
	}
	if err != nil {
		return err
	}
	if err = os.Link(path, filepath.Join(directory, name)); errors.Is(err, fs.ErrExist) {
		final := filepath.Join(directory, name)
		info, problem := os.Lstat(final)
		if problem != nil {
			return problem
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("the immutable report path is not a regular file")
		}
		data, problem := os.ReadFile(final)
		if problem != nil {
			return problem
		}
		report, problem := decodeReport(data, false)
		if problem != nil {
			return problem
		}
		if hash(report)+".json" != name {
			return fmt.Errorf("the immutable report path contains different content")
		}
		return nil
	}
	return err
}

// Save appends immutable history before atomically replacing the latest report.
// The two files are independent publications; failure is explicit and retryable.
//
//uc:realizes evidence.save
func Save(root string, report lang.RunReport) []diag.Diagnostic {
	fail := func(code, path, cause, fix string) []diag.Diagnostic {
		return []diag.Diagnostic{storeDiagnostic(code, path, cause, fix)}
	}
	normalized, err := normalizeReport(report, false)
	if err != nil {
		return fail("evidence.invalid-report", ".unicode/results.json", err.Error(), "provide a complete version 1 measured run report")
	}
	loaded := Read(root)
	if len(loaded.Diagnostics) > 0 {
		return loaded.Diagnostics
	}
	directory, problems := store(root, true)
	if len(problems) > 0 {
		return problems
	}
	latest := filepath.Join(filepath.Dir(directory), "results.json")
	if info, err := os.Lstat(latest); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fail("evidence.symlink", ".unicode/results.json", "the latest report is a symbolic link", "replace it with a regular report file")
		}
		if !info.Mode().IsRegular() {
			return fail("evidence.write-failed", ".unicode/results.json", "the latest-report path is not a regular file", "restore a regular report file")
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fail("evidence.write-failed", ".unicode/results.json", err.Error(), "restore write access to the report path")
	}
	data := append(canonical(normalized), '\n')
	if err := publish(directory, hash(normalized)+".json", data); err != nil {
		return fail("evidence.write-failed", ".unicode/runs", err.Error(), "restore write access on a filesystem supporting immutable run records and retry")
	}
	if err := maybe.WriteFile(latest, data, 0o644); err != nil {
		return fail("evidence.write-failed", ".unicode/results.json", err.Error(), "restore write access and retry saving the latest report")
	}
	return []diag.Diagnostic{}
}
