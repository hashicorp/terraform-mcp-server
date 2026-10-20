// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"encoding/json"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// projectGeneratedConfiguration extracts only simple top-level literal
// arguments from one Terraform-generated resource block. It deliberately does
// not provide variables or functions and never walks nested blocks.
func projectGeneratedConfiguration(src string) (map[string]any, error) {
	file, diagnostics := hclsyntax.ParseConfig([]byte(src), "generated.tf", hcl.Pos{Line: 1, Column: 1})
	if diagnostics.HasErrors() || file == nil {
		return nil, searchDiagnosticError(queryGeneratedConfigurationInvalidCode)
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok || len(body.Attributes) != 0 || len(body.Blocks) != 1 {
		return nil, searchDiagnosticError(queryGeneratedConfigurationInvalidCode)
	}
	block := body.Blocks[0]
	if block.Type != "resource" || len(block.Labels) != 2 {
		return nil, searchDiagnosticError(queryGeneratedConfigurationInvalidCode)
	}

	projected := make(map[string]any)
	for name, attribute := range block.Body.Attributes {
		if name == "provider" || !generatedConfigurationExpression(attribute.Expr) {
			continue
		}
		value, diagnostics := attribute.Expr.Value(nil)
		if diagnostics.HasErrors() || !value.IsKnown() || !value.IsWhollyKnown() || value.IsNull() {
			continue
		}
		unmarked, marks := value.UnmarkDeepWithPaths()
		if len(marks) != 0 {
			continue
		}
		if converted, ok := generatedConfigurationValue(unmarked); ok {
			projected[name] = converted
		}
	}
	if len(projected) == 0 {
		return nil, nil
	}
	return projected, nil
}

func generatedConfigurationExpression(expression hclsyntax.Expression) bool {
	switch expression := expression.(type) {
	case *hclsyntax.LiteralValueExpr:
		return true
	case *hclsyntax.TemplateExpr:
		return expression.IsStringLiteral()
	case *hclsyntax.ParenthesesExpr:
		return generatedConfigurationExpression(expression.Expression)
	case *hclsyntax.UnaryOpExpr:
		return expression.Op == hclsyntax.OpNegate && generatedConfigurationExpression(expression.Val)
	case *hclsyntax.ObjectConsExpr:
		if len(expression.Items) == 0 {
			return true
		}
		for _, item := range expression.Items {
			if !generatedConfigurationScalarExpression(item.ValueExpr) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func generatedConfigurationScalarExpression(expression hclsyntax.Expression) bool {
	switch expression := expression.(type) {
	case *hclsyntax.LiteralValueExpr:
		return true
	case *hclsyntax.TemplateExpr:
		return expression.IsStringLiteral()
	case *hclsyntax.ParenthesesExpr:
		return generatedConfigurationScalarExpression(expression.Expression)
	case *hclsyntax.UnaryOpExpr:
		return expression.Op == hclsyntax.OpNegate && generatedConfigurationScalarExpression(expression.Val)
	default:
		return false
	}
}

func generatedConfigurationValue(value cty.Value) (any, bool) {
	typeOf := value.Type()
	if typeOf == cty.String || typeOf == cty.Bool || typeOf == cty.Number {
		return generatedConfigurationScalar(value)
	}
	if !typeOf.IsMapType() && !typeOf.IsObjectType() || value.LengthInt() == 0 {
		return nil, false
	}

	items := make(map[string]any)
	iterator := value.ElementIterator()
	for iterator.Next() {
		key, item := iterator.Element()
		if !key.IsKnown() || key.IsNull() || !item.IsKnown() || !item.IsWhollyKnown() || item.IsNull() {
			return nil, false
		}
		unmarked, marks := item.UnmarkDeepWithPaths()
		if len(marks) != 0 {
			return nil, false
		}
		converted, ok := generatedConfigurationScalar(unmarked)
		if !ok {
			return nil, false
		}
		items[key.AsString()] = converted
	}
	return items, len(items) != 0
}

func generatedConfigurationScalar(value cty.Value) (any, bool) {
	typeOf := value.Type()
	if typeOf != cty.String && typeOf != cty.Bool && typeOf != cty.Number {
		return nil, false
	}
	raw, err := ctyjson.Marshal(value, typeOf)
	if err != nil {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var converted any
	if err := decoder.Decode(&converted); err != nil {
		return nil, false
	}
	return converted, true
}
