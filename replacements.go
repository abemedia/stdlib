package stdlib

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"os"
	"text/template"

	"golang.org/x/tools/go/analysis"
)

//nolint:gochecknoglobals
var replacements = map[string]map[string]struct {
	stdlib     string
	minVersion string
	rewrite    rewriteFunc
}{
	"github.com/samber/lo": {
		"Chunk":           {"slices.Chunk", "v1.21", nil},
		"Repeat":          {"slices.Repeat", "v1.21", chain(toSlice(0), reorderArgs(1, 0))},
		"Drop":            {"", "v1.0", tmpl("{{index .Args 0}}[{{index .Args 1}}:]")},
		"DropRight":       {"", "v1.0", tmpl("{{index .Args 0}}[:len({{index .Args 0}})-{{index .Args 1}}]")},
		"Contains":        {"slices.Contains", "v1.21", nil},
		"ContainsBy":      {"slices.ContainsFunc", "v1.21", nil},
		"IndexOf":         {"slices.Index", "v1.21", nil},
		"LastIndexOf":     {"slices.LastIndex", "v1.21", nil},
		"Min":             {"slices.Min", "v1.21", nil},
		"MinBy":           {"slices.MinFunc", "v1.21", lessToCmp(false)},
		"Max":             {"slices.Max", "v1.21", nil},
		"MaxBy":           {"slices.MaxFunc", "v1.21", lessToCmp(true)},
		"IsSorted":        {"slices.IsSorted", "v1.21", nil},
		"Flatten":         {"slices.Concat", "v1.21", toVariadic},
		"Keys":            {"maps.Keys", "v1.21", nil},
		"Values":          {"maps.Values", "v1.21", nil},
		"CoalesceOrEmpty": {"cmp.Or", "v1.21", nil},
	},
	"github.com/samber/lo/mutable": {
		"Reverse": {"slices.Reverse", "v1.21", nil},
	},
}

type rewriteFunc func(*analysis.Pass, *ast.CallExpr) ([]analysis.TextEdit, bool)

// chain returns a rewrite function that applies all given rewrite functions in order.
func chain(fn ...rewriteFunc) rewriteFunc {
	return func(pass *analysis.Pass, call *ast.CallExpr) ([]analysis.TextEdit, bool) {
		var allEdits []analysis.TextEdit
		for _, f := range fn {
			edits, ok := f(pass, call)
			if !ok {
				return nil, false
			}
			allEdits = append(allEdits, edits...)
		}
		return allEdits, true
	}
}

// tmpl returns a rewrite function that replaces the entire call with the result of executing the template.
func tmpl(templateStr string) func(pass *analysis.Pass, call *ast.CallExpr) ([]analysis.TextEdit, bool) {
	return func(pass *analysis.Pass, call *ast.CallExpr) ([]analysis.TextEdit, bool) {
		// Extract package and function names from the call (assumes a selector expression).
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return nil, false
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return nil, false
		}
		pkgStr := pkgIdent.Name
		fnStr := sel.Sel.Name

		// Gather arguments as strings using printer.Fprint.
		var args []string
		for _, arg := range call.Args {
			var buf bytes.Buffer
			if err := printer.Fprint(&buf, pass.Fset, arg); err != nil {
				return nil, false
			}
			args = append(args, buf.String())
		}

		// Data passed to the template.
		data := struct {
			Pkg  string
			Fn   string
			Args []string
		}{pkgStr, fnStr, args}

		tmpl, err := template.New("rewrite").Parse(templateStr)
		if err != nil {
			return nil, false
		}
		var out bytes.Buffer
		if err := tmpl.Execute(&out, data); err != nil {
			return nil, false
		}
		newText := out.String()

		edit := analysis.TextEdit{
			Pos:     call.Pos(),
			End:     call.End(),
			NewText: []byte(newText),
		}
		return []analysis.TextEdit{edit}, true
	}
}

// toVariadic converts the last argument of a function call to a variadic argument.
func toVariadic(_ *analysis.Pass, call *ast.CallExpr) ([]analysis.TextEdit, bool) {
	if len(call.Args) > 0 {
		arg := call.Args[len(call.Args)-1]
		edit := analysis.TextEdit{
			Pos:     arg.End(),
			End:     arg.End(),
			NewText: []byte("..."),
		}
		return []analysis.TextEdit{edit}, true
	}
	return nil, false
}

// reorderArgs returns a rewrite function that reorders the arguments of a function call according to the given order.
func reorderArgs(order ...int) func(pass *analysis.Pass, call *ast.CallExpr) ([]analysis.TextEdit, bool) {
	return func(pass *analysis.Pass, call *ast.CallExpr) ([]analysis.TextEdit, bool) {
		// Check that there are at least as many arguments as the order slice.
		if len(call.Args) < len(order) {
			return nil, false
		}

		var edits []analysis.TextEdit

		// For each position i in the order, obtain the original text for the argument at index order[i]
		// and create an edit that replaces the argument text at position i.
		for i, newIdx := range order {
			if newIdx < 0 || newIdx >= len(call.Args) {
				return nil, false
			}
			file := pass.Fset.File(call.Args[newIdx].Pos())
			if file == nil {
				return nil, false
			}
			data, err := os.ReadFile(file.Name())
			if err != nil {
				return nil, false
			}
			startOffset := file.Offset(call.Args[newIdx].Pos())
			endOffset := file.Offset(call.Args[newIdx].End())
			edits = append(edits, analysis.TextEdit{
				Pos:     call.Args[i].Pos(),
				End:     call.Args[i].End(),
				NewText: data[startOffset:endOffset],
			})
		}
		return edits, true
	}
}

// toSlice converts the argument at the specified index to a slice.
func toSlice(arg int) rewriteFunc {
	return func(pass *analysis.Pass, call *ast.CallExpr) ([]analysis.TextEdit, bool) {
		// Ensure there is an argument at the specified index.
		if len(call.Args) <= arg {
			return nil, false
		}
		expr := call.Args[arg]

		// Retrieve the type of the argument.
		tv, ok := pass.TypesInfo.Types[expr]
		if !ok || tv.Type == nil {
			return nil, false
		}
		argTypeStr := tv.Type.String()

		// Get the original source text of the argument.
		var buf bytes.Buffer
		if err := printer.Fprint(&buf, pass.Fset, expr); err != nil {
			return nil, false
		}
		originalExpr := buf.String()

		// Replace the original argument with the new text.
		edit := analysis.TextEdit{
			Pos:     expr.Pos(),
			End:     expr.End(),
			NewText: fmt.Appendf(nil, "[]%s{%s}", argTypeStr, originalExpr),
		}
		return []analysis.TextEdit{edit}, true
	}
}

// lessToCmp returns a rewrite function that converts a less function literal to a cmp function.
// If reverse is true, the comparison is reversed.
func lessToCmp(reverse bool) rewriteFunc { //nolint:gocognit
	return func(pass *analysis.Pass, call *ast.CallExpr) ([]analysis.TextEdit, bool) {
		// Ensure the last argument is a function literal.
		funcLit, ok := call.Args[len(call.Args)-1].(*ast.FuncLit)
		if !ok {
			return nil, false
		}

		var edits []analysis.TextEdit

		// Change the function literal’s result type from bool to int.
		if funcLit.Type.Results != nil && len(funcLit.Type.Results.List) > 0 {
			edits = append(edits, analysis.TextEdit{
				Pos:     funcLit.Type.Results.List[0].Type.Pos(),
				End:     funcLit.Type.Results.List[0].Type.End(),
				NewText: []byte("int"),
			})
		}

		// Process all return statements in the function literal.
		for n := range ast.Preorder(funcLit.Body) {
			retStmt, ok := n.(*ast.ReturnStmt)
			if !ok || len(retStmt.Results) != 1 {
				continue
			}
			binExpr, ok := retStmt.Results[0].(*ast.BinaryExpr)
			if !ok {
				return nil, false
			}

			// Extract source text for left and right expressions.
			left := &bytes.Buffer{}
			if err := printer.Fprint(left, pass.Fset, binExpr.X); err != nil {
				return nil, false
			}
			right := &bytes.Buffer{}
			if err := printer.Fprint(right, pass.Fset, binExpr.Y); err != nil {
				return nil, false
			}

			switch binExpr.Op {
			case token.LSS, token.LEQ:
				if reverse {
					left, right = right, left
				}
			case token.GTR, token.GEQ:
				if !reverse {
					left, right = right, left
				}
			default:
				return nil, false
			}

			edits = append(edits, analysis.TextEdit{
				Pos:     retStmt.Results[0].Pos(),
				End:     retStmt.Results[0].End(),
				NewText: fmt.Appendf(nil, "cmp.Compare(%s, %s)", left, right),
			})
		}

		return edits, true
	}
}
