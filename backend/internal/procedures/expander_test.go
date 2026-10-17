package procedures

import (
	"context"
	"strings"
	"testing"

	"voiceagent/internal/store"
)

func TestParseReferences(t *testing.T) {
	text := `Consulte @[tool:11111111-1111-1111-1111-111111111111] e depois @[procedure:22222222-2222-2222-2222-222222222222] e [kb id="kb_999"] e [system_tool id="end_call"]`
	refs := ParseReferences(text)
	if len(refs) != 4 {
		t.Fatalf("esperava 4 referências únicas, veio %d: %+v", len(refs), refs)
	}
	if refs[0].Kind != "tool" || refs[1].Kind != "procedure" || refs[2].Kind != "knowledge" || refs[3].Kind != "system_tool" {
		t.Fatalf("tipos incorretos: %+v", refs)
	}
}

func TestExpandWithCycles(t *testing.T) {
	exp := NewExpander(nil)

	procA := store.ProcedureSnapshot{
		ID:      "a",
		Name:    "Procedimento A",
		Content: `Passo 1: Chame [procedure id="b"]`,
	}
	procB := store.ProcedureSnapshot{
		ID:      "b",
		Name:    "Procedimento B",
		Content: "Passo 2: Volte para @[procedure:a]",
	}

	all := map[string]store.ProcedureSnapshot{
		"a": procA,
		"b": procB,
	}

	res, err := exp.ExpandProcedure(context.Background(), "org-1", "agent-1", procA, all)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(res.ExpandedContent, "PROCEDIMENTO INCLUÍDO: Procedimento B") {
		t.Fatal("Procedimento B não foi incluído inline")
	}
	if !strings.Contains(res.ExpandedContent, "[Procedimento já incluído]") {
		t.Fatal("Ciclo não foi quebrado")
	}
}

func TestValidateAndCompileStructuredProcedure(t *testing.T) {
	validJSON := `{
		"steps": [
			{"type": "ask", "instruction": "Pergunte o número do pedido."},
			{
				"type": "branch",
				"branches": [
					{
						"condition": {"type": "llm", "condition": "O pedido já foi enviado."},
						"steps": [{"type": "tell", "instruction": "Explique que pedidos enviados precisam ser devolvidos."}]
					}
				],
				"fallback": [
					{
						"type": "tool_call",
						"tool_id": "t1",
						"tool_name": "cancelar_pedido",
						"instruction": "Cancele o pedido.",
						"schema_overrides": {
							"notify": {"source": "constant", "constant_value": true}
						},
						"on_failure": {
							"fallback": [
								{"type": "tell", "instruction": "Houve uma falha temporária, tentando novamente."},
								{"type": "retry", "max_retries": 2}
							]
						}
					}
				]
			},
			{"type": "say", "message": "Obrigado por entrar em contato. Até logo!"},
			{"type": "system_tool", "system_tool_name": "end_call"}
		]
	}`

	toolsByID := map[string]store.Tool{
		"t1": {ID: "t1", Name: "cancelar_pedido"},
	}
	errs := ValidateStructured("proc-1", validJSON, nil, toolsByID)
	if len(errs) != 0 {
		t.Fatalf("esperava 0 erros de validação, veio: %+v", errs)
	}

	exp := NewExpander(nil)
	doc, err := ParseStructured(validJSON)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{}
	compiled, policies := exp.CompileStructured(ProcedureCompileContext{
		ToolsByID:     toolsByID,
		AllowedTools:  allowed,
		VisitedProcID: map[string]bool{},
	}, doc)

	if !allowed["cancelar_pedido"] {
		t.Fatal("esperava que cancelar_pedido fosse liberada no AllowedTools")
	}
	if policies["cancelar_pedido"].MaxRetries != 2 {
		t.Fatalf("esperava MaxRetries=2, veio %d", policies["cancelar_pedido"].MaxRetries)
	}
	if !strings.Contains(compiled, "[ASK") || !strings.Contains(compiled, "[SAY") {
		t.Fatalf("compilação incompleta: %s", compiled)
	}

	// Teste de violação: dois If (branch) seguidos e end_call fora do fim
	invalidJSON := `{
		"steps": [
			{"type": "system_tool", "system_tool_name": "end_call"},
			{"type": "branch", "branches": [{"condition": {"type": "llm", "condition": "A"}, "steps": [{"type": "tell", "instruction": "ok"}]}]},
			{"type": "branch", "branches": [{"condition": {"type": "llm", "condition": "B"}, "steps": [{"type": "tell", "instruction": "ok"}]}]}
		]
	}`
	invalidErrs := ValidateStructured("proc-2", invalidJSON, nil, nil)
	if len(invalidErrs) < 2 {
		t.Fatalf("esperava erros para end_call fora do fim e dois branch seguidos, veio: %+v", invalidErrs)
	}
}

func TestParseSOPDocument(t *testing.T) {
	sop := `# Reembolso de Pedido
Trigger: Quando o cliente pedir reembolso ou estorno de uma compra
1. Peça o número do pedido.
2. Consulte no sistema e confirme o estorno.

# Segunda Via de Boleto
1. Solicite o CPF do titular.
2. Envie a linha digitável.`

	items := ParseSOPDocument(sop)
	if len(items) != 2 {
		t.Fatalf("esperava 2 procedures extraídos do SOP, veio %d", len(items))
	}
	if items[0].Name != "Reembolso de Pedido" || items[0].Trigger != "Quando o cliente pedir reembolso ou estorno de uma compra" {
		t.Fatalf("item 0 incorreto: %+v", items[0])
	}
	if items[1].Name != "Segunda Via de Boleto" {
		t.Fatalf("item 1 incorreto: %+v", items[1])
	}
}
