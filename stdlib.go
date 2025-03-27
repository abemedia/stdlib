// Package stdlib detects uses of functions that can be replaced by the standard library.
package stdlib

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
	"golang.org/x/tools/go/analysis"
)

// NewAnalyzer creates a new analyzer that detects uses of functions that can
// be replaced by standard library functions.
func NewAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "stdlib",
		Doc:  "Detects uses of functions that can be replaced by standard library functions and suggests fixes.",
		Run: func(pass *analysis.Pass) (any, error) {
			goVersion := "v" + strings.TrimPrefix(pass.Pkg.GoVersion(), "go")
			if goVersion == "v" {
				goVersion += "1"
			}

			// candidateCount records, for each file and package, the number of candidate call expressions.
			candidateCount := make(map[*ast.File]map[string]int)

			// First pass: look for call expressions that can be replaced.
			for _, file := range pass.Files {
				processFileCalls(pass, file, goVersion, candidateCount)
			}

			// Second pass: for files that import a package from our configuration,
			// if all usages were replaced, suggest removing the import.
			processUnusedImports(pass, candidateCount)

			return nil, nil
		},
	}
}

// processFileCalls inspects a file for call expressions that can be replaced.
// It also records, per file and package, the number of candidate replacements.
func processFileCalls(
	pass *analysis.Pass,
	file *ast.File,
	modVersion string,
	candidateCount map[*ast.File]map[string]int,
) {
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		funcObj, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
		if !ok || funcObj.Pkg() == nil {
			return true
		}
		pkgPath := funcObj.Pkg().Path()
		funcName := sel.Sel.Name
		repl, found := replacements[pkgPath][funcName]
		if !found {
			return true
		}
		if semver.Compare(cmp.Or(file.GoVersion, modVersion), repl.minVersion) < 0 {
			return true
		}

		// Record candidate replacement for this package.
		if candidateCount[file] == nil {
			candidateCount[file] = make(map[string]int)
		}
		candidateCount[file][pkgPath]++

		// Replace both the package identifier and the function name.
		xIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		fixes := addReplacementTextEdit(file, xIdent, sel.Sel, repl.stdlib)

		// If the replacement has a rewrite function, apply its edits.
		if repl.rewrite != nil {
			extraEdits, ok := repl.rewrite(pass, call)
			if ok {
				fixes = append(fixes, extraEdits...)
			} else {
				fixes = nil                     // Don't suggest a fix if the rewrite failed.
				candidateCount[file][pkgPath]-- // Don't count this as a candidate.
			}
		}

		d := analysis.Diagnostic{
			Pos:     sel.Sel.Pos(),
			End:     sel.Sel.End(),
			Message: fmt.Sprintf("%s.%s can be replaced with %s", xIdent.Name, funcName, cmp.Or(repl.stdlib, "builtin")),
		}
		if len(fixes) > 0 {
			d.SuggestedFixes = []analysis.SuggestedFix{{Message: "Replace with stdlib function", TextEdits: fixes}}
		}
		pass.Report(d)

		return true
	})
}

// addReplacementTextEdit returns a slice of TextEdits that replace the package and function identifiers.
// If the package is not already imported, it also adds an import statement.
func addReplacementTextEdit(file *ast.File, pkg, fn *ast.Ident, stdlib string) []analysis.TextEdit {
	if stdlib == "" {
		return nil
	}

	stdlibPkg, stdlibFunc, ok := strings.Cut(stdlib, ".")
	if !ok {
		panic("stdlib replacement not in 'pkg.Func' form")
	}

	fixes := []analysis.TextEdit{
		{
			Pos:     pkg.Pos(),
			End:     pkg.End(),
			NewText: []byte(stdlibPkg),
		},
		{
			Pos:     fn.Pos(),
			End:     fn.End(),
			NewText: []byte(stdlibFunc),
		},
	}

	quoted := strconv.Quote(stdlibPkg)
	if slices.ContainsFunc(file.Imports, func(i *ast.ImportSpec) bool { return i.Path.Value == quoted }) {
		return fixes // Already imported.
	}

	// Look for an existing grouped import block.
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.IMPORT {
			continue
		}
		if genDecl.Lparen != token.NoPos {
			fixes = append(fixes, analysis.TextEdit{
				Pos:     genDecl.End() - 1, // before the closing ')'
				End:     genDecl.End() - 1,
				NewText: []byte("\n\t" + strconv.Quote(stdlibPkg)),
			})
		} else {
			fixes = append(fixes, analysis.TextEdit{
				Pos:     genDecl.End(),
				End:     genDecl.End(),
				NewText: []byte("\nimport " + strconv.Quote(stdlibPkg)),
			})
		}
		break
	}

	return fixes
}

// processUnusedImports checks whether a file’s import for a replaced package is no longer used,
// and if so, suggests removing it.
func processUnusedImports(pass *analysis.Pass, candidateCount map[*ast.File]map[string]int) {
	// For each file, count usage of package names.
	for _, file := range pass.Files {
		totalPkgUses := make(map[string]int)
		for _, obj := range pass.TypesInfo.Uses {
			if objPos := obj.Pos(); objPos >= file.Pos() && objPos <= file.End() {
				if pkgName, ok := obj.(*types.PkgName); ok {
					totalPkgUses[pkgName.Imported().Path()]++
				}
			}
		}
		// Check for each package in our replacement configuration if the import is now unused.
		for pkgPath := range replacements {
			var importSpec *ast.ImportSpec
			for _, imp := range file.Imports {
				if imp.Path.Value == strconv.Quote(pkgPath) {
					importSpec = imp
					break
				}
			}
			if importSpec == nil {
				continue // package not imported in this file
			}

			candidate := candidateCount[file][pkgPath]

			// If all usages are candidates for replacement and there is at least one candidate, suggest removing the import.
			if candidate > 0 && totalPkgUses[pkgPath] == candidate {
				pass.Report(analysis.Diagnostic{
					Pos:     importSpec.Pos(),
					End:     importSpec.End(),
					Message: fmt.Sprintf("The %s package import is no longer necessary", pkgPath),
					SuggestedFixes: []analysis.SuggestedFix{
						{
							Message:   "Remove unused import",
							TextEdits: []analysis.TextEdit{{Pos: importSpec.Pos(), End: importSpec.End()}},
						},
					},
				})
			}
		}
	}
}
