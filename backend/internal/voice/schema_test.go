package voice

import "testing"

func TestSanitizeToolJSONSchema(t *testing.T) {
	raw := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"$defs": map[string]any{
			"Filtro": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{
						"anyOf": []any{
							map[string]any{"type": "string", "enum": []any{"ativo", "pendente"}},
							map[string]any{"type": "null"},
						},
					},
				},
			},
		},
		"properties": map[string]any{
			"filtro": map[string]any{"$ref": "#/$defs/Filtro"},
			"tags":   map[string]any{"type": "array"},
			"dias":   map[string]any{"type": []any{"integer", "null"}},
		},
		"required": []any{"filtro"},
	}

	got := SanitizeToolJSONSchema(raw)
	if _, hasSchema := got["$schema"]; hasSchema {
		t.Fatal("esperava remoção de $schema")
	}
	if _, hasAddl := got["additionalProperties"]; hasAddl {
		t.Fatal("esperava remoção de additionalProperties")
	}
	props, ok := got["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties inválido: %#v", got["properties"])
	}
	filtro, ok := props["filtro"].(map[string]any)
	if !ok || filtro["type"] != "object" {
		t.Fatalf("ref filtro não resolvido: %#v", props["filtro"])
	}
	filtroProps := filtro["properties"].(map[string]any)
	status := filtroProps["status"].(map[string]any)
	if status["type"] != "string" {
		t.Fatalf("anyOf nullable não desembrulhado: %#v", status)
	}
	tags := props["tags"].(map[string]any)
	if _, hasItems := tags["items"]; !hasItems {
		t.Fatalf("array sem items fallback: %#v", tags)
	}
	dias := props["dias"].(map[string]any)
	if dias["type"] != "integer" {
		t.Fatalf("array type nullable não normalizado: %#v", dias)
	}
}
