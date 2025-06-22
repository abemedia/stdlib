package stdlib

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"text/template"

	"golang.org/x/tools/go/analysis"
)

//nolint:gochecknoglobals
var imports = map[string]struct {
	stdlib     string
	minVersion string
	pkgName    string
}{
	"golang.org/x/exp/maps":     {"maps", "go1.21", ""},
	"golang.org/x/exp/rand":     {"math/rand/v2", "go1.22", "rand"},
	"golang.org/x/exp/slices":   {"slices", "go1.21", ""},
	"golang.org/x/exp/slog":     {"log/slog", "go1.21", ""},
	"golang.org/x/net/context":  {"context", "go1.7", ""},
	"golang.org/x/sync/syncmap": {"sync", "go1.7", ""},
}

//nolint:gochecknoglobals
var calls = map[string]map[string]struct {
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
		"MinBy":           {"slices.MinFunc", "go1.21", lessToCmp(1, false)},
		"Max":             {"slices.Max", "go1.21", nil},
		"MaxBy":           {"slices.MaxFunc", "go1.21", lessToCmp(1, true)},
		"IsSorted":        {"slices.IsSorted", "go1.21", nil},
		"IsSortedByKey":   {"slices.IsSortedFunc", "go1.21", keyToCmp(1)},
		"Flatten":         {"slices.Concat", "go1.22", toVariadic},
		"Keys":            {"maps.Keys", "go1.23", nil},
		"Values":          {"maps.Values", "go1.23", nil},
		"CoalesceOrEmpty": {"cmp.Or", "go1.22", nil},
		"RuneLength":      {"utf8.RuneCountInString", "go1", nil},
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
		pkg := pkgIdent.Name
		fn := sel.Sel.Name

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
		}{pkg, fn, args}

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
	arg := call.Args[len(call.Args)-1]
	edit := analysis.TextEdit{Pos: arg.End(), End: arg.End(), NewText: []byte("...")}
	return []analysis.TextEdit{edit}, true
}

// lessToCmp returns a rewrite function that converts a less function literal to a cmp function.
// If reverse is true, the comparison is reversed.
func lessToCmp(arg int, reverse bool) rewriteFunc {
	return func(pass *analysis.Pass, call *ast.CallExpr) ([]analysis.TextEdit, bool) {
		// Ensure the argument is a function literal.
		funcLit, ok := call.Args[arg].(*ast.FuncLit)
		if !ok {
			return nil, false
		}

		var edits []analysis.TextEdit

		// Change the function literal’s result type from bool to int.
		edits = append(edits, analysis.TextEdit{
			Pos:     funcLit.Type.Results.List[0].Type.Pos(),
			End:     funcLit.Type.Results.List[0].Type.End(),
			NewText: []byte("int"),
		})

		// Process all return statements in the function literal.
		for n := range ast.Preorder(funcLit.Body) {
			retStmt, ok := n.(*ast.ReturnStmt)
			if !ok {
				continue
			}
			if len(retStmt.Results) != 1 {
				return nil, false
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

func keyToCmp(arg int) rewriteFunc { //nolint:funlen,gocognit
	return func(pass *analysis.Pass, call *ast.CallExpr) ([]analysis.TextEdit, bool) {
		// Ensure the argument is a function literal.
		funcLit, ok := call.Args[arg].(*ast.FuncLit)
		if !ok {
			return nil, false
		}

		var edits []analysis.TextEdit

		// Replace the current return type with "int".
		edits = append(edits, analysis.TextEdit{
			Pos:     funcLit.Type.Results.List[0].Type.Pos(),
			End:     funcLit.Type.Results.List[0].Type.End(),
			NewText: []byte("int"),
		})

		param := funcLit.Type.Params.List[0]
		if len(param.Names) != 1 {
			return nil, false
		}
		paramName := param.Names[0].Name

		// Construct the new parameter list: keep the original parameter and add ", next <type>".
		paramType := &bytes.Buffer{}
		if err := printer.Fprint(paramType, pass.Fset, param.Type); err != nil {
			return nil, false
		}
		edits = append(edits, analysis.TextEdit{
			Pos:     funcLit.Type.Params.Opening + 1,
			End:     funcLit.Type.Params.Closing,
			NewText: fmt.Appendf(nil, "%s, next %s", paramName, paramType),
		})

		// Process all return statements in the function literal.
		for n := range ast.Preorder(funcLit.Body) {
			retStmt, ok := n.(*ast.ReturnStmt)
			if !ok {
				continue
			}
			if len(retStmt.Results) != 1 {
				return nil, false
			}

			// Copy the expression and replace the original parameter with "next".
			resultBuf := &bytes.Buffer{}
			if err := printer.Fprint(resultBuf, pass.Fset, retStmt.Results[0]); err != nil {
				return nil, false
			}
			result := resultBuf.String()
			exprNext, err := parser.ParseExpr(result)
			if err != nil {
				return nil, false
			}
			var hasParam bool
			ast.Inspect(exprNext, func(n ast.Node) bool {
				if ident, ok := n.(*ast.Ident); ok && ident.Name == paramName {
					ident.Name = "next"
					hasParam = true
				}
				return true
			})
			if !hasParam {
				return nil, false
			}

			next := &bytes.Buffer{}
			if err := printer.Fprint(next, pass.Fset, exprNext); err != nil {
				return nil, false
			}

			edits = append(edits, analysis.TextEdit{
				Pos:     retStmt.Results[0].Pos(),
				End:     retStmt.Results[0].End(),
				NewText: fmt.Appendf(nil, "cmp.Compare(%s, %s)", result, next),
			})
		}

		return edits, true
	}
}
