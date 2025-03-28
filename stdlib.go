// Package stdlib detects uses of functions that can be replaced by the standard library.
package stdlib

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"go/version"
	"path"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// NewAnalyzer creates a new analyzer that detects uses of functions that can
// be replaced by standard library functions.
func NewAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "stdlib",
		Doc:  "Detects uses of functions that can be replaced by standard library functions and suggests fixes.",
		Run: func(pass *analysis.Pass) (any, error) {
			// references records, for each file and package, the number of candidate call expressions.
			references := make(map[*ast.File]map[string]int)

			// Replace expressions in each file.
			for _, file := range pass.Files {
				processFileCalls(pass, file, references)
			}

			// Remove unused imports.
			processUnusedImports(pass, references)

			return nil, nil
		},
	}
}

// processFileCalls inspects a file for call expressions that can be replaced.
// It also records, per file and package, the number of references to each package.
func processFileCalls(pass *analysis.Pass, file *ast.File, references map[*ast.File]map[string]int) {
	goVersion := cmp.Or(file.GoVersion, pass.Pkg.GoVersion(), "go1.9999")

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
		if version.Compare(goVersion, repl.minVersion) < 0 {
			return true
		}

		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		// Record references to this package using the local package name.
		if references[file] == nil {
			references[file] = make(map[string]int)
		}
		references[file][pkg.Name]++

		fixes := addReplacementTextEdit(file, pkg, sel.Sel, repl.stdlib)

		// If the replacement has a rewrite function, apply its edits.
		if repl.rewrite != nil {
			edits, ok := repl.rewrite(pass, call)
			if ok {
				fixes = append(fixes, edits...)
			} else {
				fixes = nil                  // Don't suggest a fix if the rewrite failed.
				references[file][pkg.Name]-- // Don't count this as a candidate.
			}
		}

		d := analysis.Diagnostic{
			Pos:     sel.Sel.Pos(),
			End:     sel.Sel.End(),
			Message: fmt.Sprintf("%s.%s can be replaced with %s", path.Base(pkgPath), funcName, cmp.Or(repl.stdlib, "builtin")),
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
		{Pos: pkg.Pos(), End: pkg.End(), NewText: []byte(stdlibPkg)},
		{Pos: fn.Pos(), End: fn.End(), NewText: []byte(stdlibFunc)},
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
func processUnusedImports(pass *analysis.Pass, references map[*ast.File]map[string]int) {
	// For each file, count usage of package names using the local alias.
	for _, file := range pass.Files {
		totalPkgUses := make(map[string]int)
		for _, obj := range pass.TypesInfo.Uses {
			if objPos := obj.Pos(); objPos >= file.Pos() && objPos <= file.End() {
				if pkgName, ok := obj.(*types.PkgName); ok {
					totalPkgUses[pkgName.Name()]++
				}
			}
		}

		// Check each import spec in the file.
		for _, importSpec := range file.Imports {
			pkgPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				continue
			}
			// Only consider packages that have a replacement configured.
			if _, ok := replacements[pkgPath]; !ok {
				continue
			}
			// Determine the local alias: if a name is provided, use it; otherwise,
			// use the package's default (the base of the path).
			var localAlias string
			if importSpec.Name != nil {
				localAlias = importSpec.Name.Name
			} else {
				localAlias = path.Base(pkgPath)
			}

			candidateCount := references[file][localAlias]

			// If all usages for this alias are candidates for replacement and there
			// is at least one candidate, suggest removing the import.
			if candidateCount > 0 && totalPkgUses[localAlias] == candidateCount {
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
