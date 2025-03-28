package stdlib

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
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
		"Chunk":           {"slices.Chunk", "go1.23", nil},
		"Drop":            {"", "go1", tmpl("{{index .Args 0}}[{{index .Args 1}}:]")},
		"DropRight":       {"", "go1", tmpl("{{index .Args 0}}[:len({{index .Args 0}})-{{index .Args 1}}]")},
		"Contains":        {"slices.Contains", "go1.21", nil},
		"ContainsBy":      {"slices.ContainsFunc", "go1.21", nil},
		"IndexOf":         {"slices.Index", "go1.21", nil},
		"Min":             {"slices.Min", "go1.21", nil},
		"MinBy":           {"slices.MinFunc", "go1.21", lessToCmp(false)},
		"Max":             {"slices.Max", "go1.21", nil},
		"MaxBy":           {"slices.MaxFunc", "go1.21", lessToCmp(true)},
		"IsSorted":        {"slices.IsSorted", "go1.21", nil},
		"Flatten":         {"slices.Concat", "go1.22", toVariadic},
		"Keys":            {"maps.Keys", "go1.23", nil},
		"Values":          {"maps.Values", "go1.23", nil},
		"CoalesceOrEmpty": {"cmp.Or", "go1.22", nil},
	},
	"github.com/samber/lo/mutable": {
		"Reverse": {"slices.Reverse", "go1.21", nil},
	},
}

type rewriteFunc func(*analysis.Pass, *ast.CallExpr) ([]analysis.TextEdit, bool)

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
		edit := analysis.TextEdit{Pos: arg.End(), End: arg.End(), NewText: []byte("...")}
		return []analysis.TextEdit{edit}, true
	}
	return nil, false
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
