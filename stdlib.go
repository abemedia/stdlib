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
	"golang.org/x/tools/go/ast/astutil"
)

// NewAnalyzer creates a new analyzer that detects uses of functions that can
// be replaced by standard library functions.
func NewAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "stdlib",
		Doc:  "Detects uses of functions that can be replaced by standard library functions and suggests fixes.",
		Run: func(pass *analysis.Pass) (any, error) {
			// references records, for each file and package, the number of candidates.
			references := make(map[*ast.File]map[string]int)

			for _, file := range pass.Files {
				references[file] = make(map[string]int)
				processFileImports(pass, file)
				processFileSymbols(pass, file, references)
				processUnusedImports(pass, file, references)
			}

			return nil, nil
		},
	}
}

// processFileImports inspects a file for package imports that can be replaced.
func processFileImports(pass *analysis.Pass, file *ast.File) {
	goVersion := cmp.Or(file.GoVersion, pass.Pkg.GoVersion(), "go1.9999")

	for _, importSpec := range file.Imports {
		pkgPath, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil {
			continue
		}

		pkgRepl, ok := imports[pkgPath]
		if !ok || version.Compare(goVersion, pkgRepl.minVersion) < 0 {
			continue
		}

		pkgName := pass.TypesInfo.PkgNameOf(importSpec)
		oldAlias := pkgName.String()

		// Determine the alias to be used for the new package.
		// If an explicit alias was used, we keep it. Otherwise, we use the new default.
		newAlias := oldAlias
		if importSpec.Name == nil {
			newAlias = cmp.Or(pkgRepl.pkgName, path.Base(pkgRepl.stdlib))
		}

		// Create TextEdit to replace the import path.
		edits := []analysis.TextEdit{
			{Pos: importSpec.Path.Pos(), End: importSpec.Path.End(), NewText: []byte(strconv.Quote(pkgRepl.stdlib))},
		}

		// Add TextEdits for renaming the alias if it changes.
		if oldAlias != newAlias {
			for ident, obj := range pass.TypesInfo.Uses {
				if obj == pkgName {
					edits = append(edits, analysis.TextEdit{Pos: ident.Pos(), End: ident.End(), NewText: []byte(newAlias)})
				}
			}
		}

		pass.Report(analysis.Diagnostic{
			Category: "stdlib",
			Pos:      importSpec.Pos(),
			End:      importSpec.End(),
			Message:  fmt.Sprintf("Package %q can be replaced with %q", pkgPath, pkgRepl.stdlib),
			SuggestedFixes: []analysis.SuggestedFix{
				{Message: "Replace package import and update references", TextEdits: edits},
			},
		})
	}
}

// processFileSymbols inspects a file for functions and types that can be replaced.
// It also records, per file and package, the number of references to each package.
func processFileSymbols(pass *analysis.Pass, file *ast.File, references map[*ast.File]map[string]int) {
	goVersion := cmp.Or(file.GoVersion, pass.Pkg.GoVersion(), "go1.9999")

	for ident, obj := range pass.TypesInfo.Uses {
		if ident.Pos() < file.Pos() || ident.End() > file.End() || obj.Pkg() == nil {
			continue
		}

		repl, ok := symbols[obj.Pkg().Path()][obj.Name()]
		if !ok || version.Compare(goVersion, repl.minVersion) < 0 {
			continue
		}

		var edits []analysis.TextEdit
		{
			// Find the enclosing call expression for the function identifier.
			// The path is expected to be ident -> SelectorExpr -> CallExpr.
			pathNodes, _ := astutil.PathEnclosingInterval(file, ident.Pos(), ident.End())
			if len(pathNodes) < 3 {
				goto Report
			}
			sel, isSel := pathNodes[1].(*ast.SelectorExpr)
			call, isCall := pathNodes[2].(*ast.CallExpr)
			_, isField := pathNodes[2].(*ast.Field)
			if !isSel || sel.Sel != ident || (!isCall && !isField) {
				goto Report
			}

			edits = addReplacementTextEdit(file, sel.X, sel.Sel, repl.stdlib)
			if isCall && repl.rewrite != nil {
				if rewrite, ok := repl.rewrite(pass, call); ok {
					edits = append(edits, rewrite...)
				} else {
					edits = nil // Don't suggest a fix if the rewrite failed.
				}
			}
			if len(edits) > 0 {
				references[file][sel.X.(*ast.Ident).Name]++ // Record references to this package using the local package name.
			}
		}

	Report:
		pass.Report(analysis.Diagnostic{
			Category:       "stdlib",
			Pos:            ident.Pos(),
			End:            ident.End(),
			Message:        fmt.Sprintf("%s.%s can be replaced with %s", obj.Pkg().Path(), obj.Name(), cmp.Or(repl.stdlib, "builtin")),
			SuggestedFixes: []analysis.SuggestedFix{{Message: "Replace with stdlib", TextEdits: edits}},
		})
	}
}

// addReplacementTextEdit returns a slice of TextEdits that replace the package and function identifiers.
// If the package is not already imported, it also adds an import statement.
func addReplacementTextEdit(file *ast.File, pkg, fn ast.Node, stdlib string) []analysis.TextEdit {
	if stdlib == "" {
		return nil
	}

	stdlibPkg, stdlibSymbol, ok := strings.Cut(stdlib, ".")
	if !ok {
		panic("stdlib replacement not in 'pkg.Symbol' form")
	}

	edits := []analysis.TextEdit{
		{Pos: pkg.Pos(), End: pkg.End(), NewText: []byte(stdlibPkg)},
		{Pos: fn.Pos(), End: fn.End(), NewText: []byte(stdlibSymbol)},
	}

	quoted := strconv.Quote(stdlibPkg)
	if slices.ContainsFunc(file.Imports, func(i *ast.ImportSpec) bool { return i.Path.Value == quoted }) {
		return edits // Already imported.
	}

	// Look for an existing grouped import block.
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.IMPORT {
			continue
		}
		if genDecl.Lparen != token.NoPos {
			edits = append(edits, analysis.TextEdit{
				Pos:     genDecl.End() - 1, // before the closing ')'
				End:     genDecl.End() - 1,
				NewText: []byte("\n\t" + strconv.Quote(stdlibPkg)),
			})
		} else {
			edits = append(edits, analysis.TextEdit{
				Pos:     genDecl.End(),
				End:     genDecl.End(),
				NewText: []byte("\nimport " + strconv.Quote(stdlibPkg)),
			})
		}
		break
	}

	return edits
}

// processUnusedImports checks whether a file’s import for a replaced package is no longer used,
// and if so, suggests removing it.
func processUnusedImports(pass *analysis.Pass, file *ast.File, references map[*ast.File]map[string]int) {
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
		if _, ok := symbols[pkgPath]; !ok {
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
				Category: "unused",
				Pos:      importSpec.Pos(),
				End:      importSpec.End(),
				Message:  fmt.Sprintf("The %s package import is no longer necessary", pkgPath),
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
