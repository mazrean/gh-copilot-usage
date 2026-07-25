// Command lint bundles go vet's default analyzers together with
// staticcheck and stylecheck into a single multichecker binary.
// Run it via `go tool lint ./...` from the repo root.
package main

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
	"golang.org/x/tools/go/analysis/passes/appends"
	"golang.org/x/tools/go/analysis/passes/asmdecl"
	"golang.org/x/tools/go/analysis/passes/assign"
	"golang.org/x/tools/go/analysis/passes/atomic"
	"golang.org/x/tools/go/analysis/passes/bools"
	"golang.org/x/tools/go/analysis/passes/buildtag"
	"golang.org/x/tools/go/analysis/passes/cgocall"
	"golang.org/x/tools/go/analysis/passes/composite"
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/passes/directive"
	"golang.org/x/tools/go/analysis/passes/errorsas"
	"golang.org/x/tools/go/analysis/passes/framepointer"
	"golang.org/x/tools/go/analysis/passes/httpresponse"
	"golang.org/x/tools/go/analysis/passes/ifaceassert"
	"golang.org/x/tools/go/analysis/passes/loopclosure"
	"golang.org/x/tools/go/analysis/passes/lostcancel"
	"golang.org/x/tools/go/analysis/passes/modernize"
	"golang.org/x/tools/go/analysis/passes/nilfunc"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/shift"
	"golang.org/x/tools/go/analysis/passes/sigchanyzer"
	"golang.org/x/tools/go/analysis/passes/slog"
	"golang.org/x/tools/go/analysis/passes/stdmethods"
	"golang.org/x/tools/go/analysis/passes/stringintconv"
	"golang.org/x/tools/go/analysis/passes/structtag"
	"golang.org/x/tools/go/analysis/passes/testinggoroutine"
	"golang.org/x/tools/go/analysis/passes/tests"
	"golang.org/x/tools/go/analysis/passes/timeformat"
	"golang.org/x/tools/go/analysis/passes/unmarshal"
	"golang.org/x/tools/go/analysis/passes/unreachable"
	"golang.org/x/tools/go/analysis/passes/unsafeptr"
	"golang.org/x/tools/go/analysis/passes/unusedresult"
	"golang.org/x/tools/go/analysis/passes/waitgroup"
	"honnef.co/go/tools/staticcheck"
	"honnef.co/go/tools/stylecheck"
)

// vetAnalyzers mirrors the analyzer set `go vet` runs by default.
var vetAnalyzers = []*analysis.Analyzer{
	appends.Analyzer, asmdecl.Analyzer, assign.Analyzer, atomic.Analyzer,
	bools.Analyzer, buildtag.Analyzer, cgocall.Analyzer, composite.Analyzer,
	copylock.Analyzer, directive.Analyzer, errorsas.Analyzer, framepointer.Analyzer,
	httpresponse.Analyzer, ifaceassert.Analyzer, loopclosure.Analyzer, lostcancel.Analyzer,
	nilfunc.Analyzer, printf.Analyzer, shift.Analyzer, sigchanyzer.Analyzer, slog.Analyzer,
	stdmethods.Analyzer, stringintconv.Analyzer, structtag.Analyzer, testinggoroutine.Analyzer,
	tests.Analyzer, timeformat.Analyzer, unmarshal.Analyzer, unreachable.Analyzer,
	unsafeptr.Analyzer, unusedresult.Analyzer, waitgroup.Analyzer,
}

func main() {
	analyzers := append([]*analysis.Analyzer{}, vetAnalyzers...)
	for _, a := range staticcheck.Analyzers {
		analyzers = append(analyzers, a.Analyzer)
	}
	for _, a := range stylecheck.Analyzers {
		analyzers = append(analyzers, a.Analyzer)
	}
	analyzers = append(analyzers, modernize.Suite...)
	for i, a := range analyzers {
		analyzers[i] = skipGenerated(a)
	}
	multichecker.Main(analyzers...)
}

// skipGenerated wraps an analyzer so it doesn't report diagnostics in files
// carrying the standard "Code generated ... DO NOT EDIT." header (e.g.
// templ/kessoku output) — those files are regenerated from source, not
// authored by hand. Diagnostics are filtered at report time rather than by
// pre-trimming pass.Files, since some checks (e.g. staticcheck's SSA-based
// analyzers) derive their findings from a Requires dependency built over the
// full, unfiltered file set.
func skipGenerated(a *analysis.Analyzer) *analysis.Analyzer {
	run := a.Run
	wrapped := *a
	wrapped.Run = func(pass *analysis.Pass) (any, error) {
		generated := make(map[*token.File]bool)
		for _, f := range pass.Files {
			if isGenerated(f) {
				generated[pass.Fset.File(f.Pos())] = true
			}
		}
		report := pass.Report
		newPass := *pass
		newPass.Report = func(d analysis.Diagnostic) {
			if generated[pass.Fset.File(d.Pos)] {
				return
			}
			report(d)
		}
		return run(&newPass)
	}
	return &wrapped
}

func isGenerated(f *ast.File) bool {
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.Contains(c.Text, "Code generated") && strings.Contains(c.Text, "DO NOT EDIT") {
				return true
			}
		}
	}
	return false
}
