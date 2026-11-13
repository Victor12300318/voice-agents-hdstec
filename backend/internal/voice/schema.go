package voice

import (
	"fmt"
	"strings"
)

// SanitizeToolJSONSchema normaliza um JSON Schema arbitrário (ex.: vindo de MCP tools/list)
// para um formato compatível tanto com Gemini Live quanto com OpenAI Realtime / GPT-Live.
func SanitizeToolJSONSchema(raw map[string]any) map[string]any {
	if len(raw) == 0 {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		}
	}

	defs := map[string]any{}
	for _, key := range []string{"$defs", "definitions"} {
		if m, ok := raw[key].(map[string]any); ok {
			for k, v := range m {
				defs[k] = v
			}
		}
	}

	cleaned := sanitizeSchemaNode(raw, defs, 0)
	if cleaned == nil {
		cleaned = map[string]any{}
	}
	if _, ok := cleaned["type"]; !ok {
		cleaned["type"] = "object"
	}
	if cleaned["type"] == "object" {
		if _, ok := cleaned["properties"]; !ok {
			cleaned["properties"] = map[string]any{}
		}
	}
	return cleaned
}

func sanitizeSchemaNode(node map[string]any, defs map[string]any, depth int) map[string]any {
	if node == nil || depth > 12 {
		return map[string]any{"type": "string"}
	}

	// Resolve local $ref (#/$defs/X ou #/definitions/X)
	if ref, ok := node["$ref"].(string); ok && ref != "" {
		if target := resolveLocalRef(ref, defs); target != nil {
			merged := make(map[string]any, len(target)+len(node))
			for k, v := range target {
				merged[k] = v
			}
			for k, v := range node {
				if k != "$ref" {
					merged[k] = v
				}
			}
			return sanitizeSchemaNode(merged, defs, depth+1)
		}
	}

	// Desembrulha anyOf / oneOf com branch null (ex.: Zod .nullable() / Pydantic Optional)
	for _, unionKey := range []string{"anyOf", "oneOf"} {
		if arr, ok := node[unionKey].([]any); ok && len(arr) > 0 {
			var chosen map[string]any
			for _, item := range arr {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if t, _ := m["type"].(string); t == "null" {
					continue
				}
				chosen = m
				break
			}
			if chosen != nil {
				merged := make(map[string]any, len(node)+len(chosen))
				for k, v := range node {
					if k != "anyOf" && k != "oneOf" {
						merged[k] = v
					}
				}
				for k, v := range chosen {
					if _, exists := merged[k]; !exists || k == "type" || k == "properties" || k == "items" || k == "enum" {
						merged[k] = v
					}
				}
				return sanitizeSchemaNode(merged, defs, depth+1)
			}
		}
	}

	out := make(map[string]any, len(node))
	for k, v := range node {
		switch k {
		case "$schema", "$id", "$comment", "$defs", "definitions", "additionalProperties":
			continue
		case "type":
			switch tv := v.(type) {
			case string:
				if tv == "null" {
					out["type"] = "string"
				} else {
					out["type"] = tv
				}
			case []any:
				picked := "string"
				for _, item := range tv {
					if s, ok := item.(string); ok && s != "" && s != "null" {
						picked = s
						break
					}
				}
				out["type"] = picked
			case []string:
				picked := "string"
				for _, s := range tv {
					if s != "" && s != "null" {
						picked = s
						break
					}
				}
				out["type"] = picked
			}
		case "properties":
			if props, ok := v.(map[string]any); ok {
				cleanProps := make(map[string]any, len(props))
				for pk, pv := range props {
					if pm, ok := pv.(map[string]any); ok {
						cleanProps[pk] = sanitizeSchemaNode(pm, defs, depth+1)
					} else {
						cleanProps[pk] = map[string]any{"type": "string"}
					}
				}
				out["properties"] = cleanProps
			}
		case "items":
			if im, ok := v.(map[string]any); ok {
				out["items"] = sanitizeSchemaNode(im, defs, depth+1)
			} else {
				out["items"] = map[string]any{"type": "string"}
			}
		case "required":
			switch rv := v.(type) {
			case []string:
				out["required"] = rv
			case []any:
				req := make([]string, 0, len(rv))
				for _, item := range rv {
					if s, ok := item.(string); ok && s != "" {
						req = append(req, s)
					}
				}
				out["required"] = req
			}
		default:
			out[k] = v
		}
	}

	if _, hasType := out["type"]; !hasType {
		if _, hasProps := out["properties"]; hasProps {
			out["type"] = "object"
		} else if _, hasItems := out["items"]; hasItems {
			out["type"] = "array"
		} else {
			out["type"] = "string"
		}
	}
	if out["type"] == "object" {
		if _, ok := out["properties"]; !ok {
			out["properties"] = map[string]any{}
		}
	}
	if out["type"] == "array" {
		if _, ok := out["items"]; !ok {
			out["items"] = map[string]any{"type": "string"}
		}
	}
	return out
}

func resolveLocalRef(ref string, defs map[string]any) map[string]any {
	for _, prefix := range []string{"#/$defs/", "#/definitions/"} {
		if strings.HasPrefix(ref, prefix) {
			name := strings.TrimPrefix(ref, prefix)
			if m, ok := defs[name].(map[string]any); ok {
				return m
			}
		}
	}
	return nil
}

// ExtractFlatParams converte propriedades de nível superior de um JSON Schema em []Param.
func ExtractFlatParams(schema map[string]any) []Param {
	if len(schema) == 0 {
		return nil
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok || len(props) == 0 {
		return nil
	}
	reqSet := map[string]bool{}
	switch rv := schema["required"].(type) {
	case []string:
		for _, r := range rv {
			reqSet[r] = true
		}
	case []any:
		for _, r := range rv {
			reqSet[fmt.Sprint(r)] = true
		}
	}

	out := make([]Param, 0, len(props))
	for name, rawProp := range props {
		pm, _ := rawProp.(map[string]any)
		ptype := "string"
		desc := ""
		if pm != nil {
			if s, ok := pm["type"].(string); ok && s != "" {
				ptype = s
			}
			if d, ok := pm["description"].(string); ok {
				desc = d
			}
		}
		out = append(out, Param{
			Name:        name,
			Type:        ptype,
			Description: desc,
			Required:    reqSet[name],
		})
	}
	return out
}
