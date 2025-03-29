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
			// usage records the number of times the package is used in the file.
			// This is used to determine if an import can be removed.
			usage := make(map[*ast.File]map[[2]string]int)

			// Replace expressions in each file.
			for _, file := range pass.Files {
				processFileCalls(pass, file, usage)
			}

			// Remove unused imports.
			processUnusedImports(pass, usage)

			return nil, nil
		},
	}
}

// processFileCalls inspects a file for call expressions that can be replaced.
// It also records, per file and package, the number of references to each package.
func processFileCalls(pass *analysis.Pass, file *ast.File, usage map[*ast.File]map[[2]string]int) {
	goVersion := cmp.Or(file.GoVersion, pass.Pkg.GoVersion(), "go1.9999")

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		var funcObj *types.Func
		var pkgPath, funcName, localAlias string
		switch expr := call.Fun.(type) {
		case *ast.SelectorExpr: // Qualified call: pkg.Func
			ident, ok := expr.X.(*ast.Ident)
			if !ok {
				return true
			}
			localAlias = ident.Name
			obj, ok := pass.TypesInfo.Uses[expr.Sel].(*types.Func)
			if !ok || obj.Pkg() == nil {
				return true
			}
			funcObj = obj
			pkgPath = funcObj.Pkg().Path()
			funcName = expr.Sel.Name
		case *ast.Ident: // Dot import usage: Func (source uses an unqualified identifier)
			obj, ok := pass.TypesInfo.Uses[expr].(*types.Func)
			if !ok || obj.Pkg() == nil {
				return true
			}
			funcObj = obj
			pkgPath = funcObj.Pkg().Path()
			funcName = expr.Name
			localAlias = "."
		default:
			return true
		}

		if _, pkgTracked := replacements[pkgPath]; !pkgTracked {
			return true
		}

		// Record usage of this package.
		key := [2]string{localAlias, pkgPath}
		if usage[file] == nil {
			usage[file] = make(map[[2]string]int)
		}
		usage[file][key]++

		repl, found := replacements[pkgPath][funcName]
		if !found || version.Compare(goVersion, repl.minVersion) < 0 {
			return true
		}

		var fixes []analysis.TextEdit
		switch expr := call.Fun.(type) {
		case *ast.SelectorExpr:
			pkg, ok := expr.X.(*ast.Ident)
			if !ok {
				return true
			}
			fixes = addReplacementTextEdit(file, pkg, expr.Sel, repl.stdlib)
		case *ast.Ident:
			fixes = addReplacementTextEdit(file, nil, expr, repl.stdlib)
		}

		// If a rewrite function is provided, try to use it.
		if repl.rewrite != nil {
			edits, ok := repl.rewrite(pass, call)
			if ok {
				fixes = append(fixes, edits...)
				// Successful replacement: decrement usage.
				usage[file][key]--
			} else {
				// Replacement failed; do not modify usage.
				fixes = nil
			}
		} else {
			// No rewrite function means a straightforward replacement.
			usage[file][key]--
		}

		d := analysis.Diagnostic{
			Pos:     call.Fun.Pos(),
			End:     call.Fun.End(),
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

	var fixes []analysis.TextEdit
	if pkg != nil {
		// Qualified usage: replace both the qualifier and the function.
		fixes = []analysis.TextEdit{
			{Pos: pkg.Pos(), End: pkg.End(), NewText: []byte(stdlibPkg)},
			{Pos: fn.Pos(), End: fn.End(), NewText: []byte(stdlibFunc)},
		}
	} else {
		// Dot import usage: replace the unqualified function with a qualified call.
		fixes = []analysis.TextEdit{
			{Pos: fn.Pos(), End: fn.End(), NewText: []byte(stdlibPkg + "." + stdlibFunc)},
		}
	}

	// Add an import for stdlibPkg if it's not already present.
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
				NewText: []byte("\n\t" + quoted),
			})
		} else {
			fixes = append(fixes, analysis.TextEdit{
				Pos:     genDecl.End(),
				End:     genDecl.End(),
				NewText: []byte("\nimport " + quoted),
			})
		}
		break
	}

	return fixes
}

// processUnusedImports checks whether a file’s import for a replaced package is no longer used,
// and if so, suggests removing it.
func processUnusedImports(pass *analysis.Pass, usage map[*ast.File]map[[2]string]int) {
	for _, file := range pass.Files {
		for _, importSpec := range file.Imports {
			pkgPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				continue
			}
			// Only consider imports for packages that have a replacement configuration.
			if _, ok := replacements[pkgPath]; !ok {
				continue
			}
			// Determine the effective local alias as it appears in source:
			// if a name is provided, use it; otherwise, use the default (base of pkgPath).
			var effectiveAlias string
			if importSpec.Name != nil {
				effectiveAlias = importSpec.Name.Name
			} else {
				effectiveAlias = path.Base(pkgPath)
			}

			// If the package is not used in the file, suggest removing the import.
			if usage[file][[2]string{effectiveAlias, pkgPath}] == 0 {
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
