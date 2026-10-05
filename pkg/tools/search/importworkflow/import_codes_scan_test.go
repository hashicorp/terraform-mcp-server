// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var importCodePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)+`)

// emittedImportCodes parses the package's non-test sources and returns every
// diagnostic or note code written as a string literal: arguments to the code
// constructors, values appended to a Diagnostics or Notes field, Diagnostics
// composite literals, and the codes returned by importSchemaHTTPStatus. A code
// built dynamically is not found, so emit codes as literals.
func emittedImportCodes(t *testing.T) map[string][]string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	found := map[string][]string{}
	add := func(file string, lit ast.Expr) {
		basic, ok := lit.(*ast.BasicLit)
		if !ok || basic.Kind != token.STRING {
			return
		}
		value, err := strconv.Unquote(basic.Value)
		if err != nil || !importCodePattern.MatchString(value) {
			return
		}
		code := importCodePattern.FindString(value)
		if code != value && !strings.HasPrefix(value, code+";") {
			return
		}
		found[code] = append(found[code], file)
	}
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		require.NoError(t, err)
		ast.Inspect(parsed, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if id, ok := x.Fun.(*ast.Ident); ok {
					switch id.Name {
					case "importEvidenceFailure", "importCreateBlocked":
						if len(x.Args) > 0 {
							add(file, x.Args[0])
						}
					case "append":
						if sel, ok := x.Args[0].(*ast.SelectorExpr); ok && (sel.Sel.Name == "Diagnostics" || sel.Sel.Name == "Notes") {
							for _, arg := range x.Args[1:] {
								add(file, arg)
							}
						}
					}
				}
			case *ast.KeyValueExpr:
				if key, ok := x.Key.(*ast.Ident); ok && key.Name == "Diagnostics" {
					if lit, ok := x.Value.(*ast.CompositeLit); ok {
						for _, elt := range lit.Elts {
							add(file, elt)
						}
					}
				}
			case *ast.FuncDecl:
				if x.Name.Name == "importSchemaHTTPStatus" {
					ast.Inspect(x, func(m ast.Node) bool {
						if ret, ok := m.(*ast.ReturnStmt); ok {
							for _, r := range ret.Results {
								add(file, r)
							}
						}
						return true
					})
				}
			}
			return true
		})
	}
	return found
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
