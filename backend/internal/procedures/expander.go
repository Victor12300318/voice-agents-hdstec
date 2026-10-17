// Package procedures implementa a resolução, expansão inline de referências,
// compilação de Structured Procedures e allowlist de ferramentas (ADR 0001).
package procedures

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"voiceagent/internal/store"
	"voiceagent/internal/variables"
)

// Suporta tanto o formato interno @[kind:id] quanto a sintaxe oficial da ElevenLabs [tool id="..."], [kb id="..."], [procedure id="..."], [system_tool id="..."]
var (
	refTokenRe      = regexp.MustCompile(`@\[(tool|procedure|knowledge|system_tool):([^\]]+)\]`)
	elevenRefTokenRe = regexp.MustCompile(`\[(tool|kb|knowledge|procedure|system_tool)\s+id="([^"]+)"\]`)
)

type Reference struct {
	Kind string // tool | procedure | knowledge | system_tool
	ID   string
}

func normalizeKind(k string) string {
	if k == "kb" {
		return "knowledge"
	}
	return k
}

// ParseReferences extrai todas as referências inline (tanto @[kind:id] quanto [kind id="..."]) de um texto.
func ParseReferences(text string) []Reference {
	var out []Reference
	seen := map[string]bool{}
	add := func(kind, id string) {
		kind = normalizeKind(kind)
		key := kind + ":" + id
		if !seen[key] && id != "" {
			seen[key] = true
			out = append(out, Reference{Kind: kind, ID: id})
		}
	}

	for _, m := range refTokenRe.FindAllStringSubmatch(text, -1) {
		add(m[1], m[2])
	}
	for _, m := range elevenRefTokenRe.FindAllStringSubmatch(text, -1) {
		add(m[1], m[2])
	}
	return out
}

type ProcedureMetadata struct {
	IsSubProcedure             bool              `json:"is_sub_procedure"`
	ReferencedToolIDs          []string          `json:"referenced_tool_ids"`
	ReferencedKBIDs            []string          `json:"referenced_kb_ids"`
	ReferencedProcedureIDs     []string          `json:"referenced_procedure_ids"`
	ReferencedDynamicVariables []string          `json:"referenced_dynamic_variables"`
	ValidationErrors           []ValidationError `json:"validation_errors"`
}

// InspectProcedure analisa referências inline, variáveis dinâmicas e erros de validação/referências quebradas (estilo ElevenLabs).
func InspectProcedure(
	p store.Procedure,
	allProcs map[string]store.ProcedureSnapshot,
	toolsByID map[string]store.Tool,
	kbsByID map[string]store.KnowledgeBase,
) ProcedureMetadata {
	meta := ProcedureMetadata{
		IsSubProcedure:             strings.TrimSpace(p.Trigger) == "",
		ReferencedToolIDs:          []string{},
		ReferencedKBIDs:            []string{},
		ReferencedProcedureIDs:     []string{},
		ReferencedDynamicVariables: variables.References(p.Trigger, p.Content),
		ValidationErrors:           []ValidationError{},
	}
	if meta.ReferencedDynamicVariables == nil {
		meta.ReferencedDynamicVariables = []string{}
	}

	if p.Type == "deterministic" {
		errs := ValidateStructured(p.ID, p.Content, allProcs, toolsByID)
		meta.ValidationErrors = append(meta.ValidationErrors, errs...)
		if doc, err := ParseStructured(p.Content); err == nil {
			collectStepRefs(doc.Steps, &meta)
		}
		return meta
	}

	// Free-form procedure: inspeciona referências no trigger e no content
	refs := ParseReferences(p.Trigger + "\n" + p.Content)
	for _, ref := range refs {
		switch ref.Kind {
		case "tool":
			meta.ReferencedToolIDs = append(meta.ReferencedToolIDs, ref.ID)
			if len(toolsByID) > 0 {
				if _, ok := toolsByID[ref.ID]; !ok {
					meta.ValidationErrors = append(meta.ValidationErrors, ValidationError{
						Path:    "content.tool[" + ref.ID + "]",
						Message: fmt.Sprintf("Referência inválida: a Tool %q não existe mais ou foi removida.", ref.ID),
					})
				}
			}
		case "knowledge":
			meta.ReferencedKBIDs = append(meta.ReferencedKBIDs, ref.ID)
			if len(kbsByID) > 0 {
				if _, ok := kbsByID[ref.ID]; !ok {
					meta.ValidationErrors = append(meta.ValidationErrors, ValidationError{
						Path:    "content.kb[" + ref.ID + "]",
						Message: fmt.Sprintf("Referência inválida: a Knowledge Base %q não existe mais.", ref.ID),
					})
				}
			}
		case "procedure":
			meta.ReferencedProcedureIDs = append(meta.ReferencedProcedureIDs, ref.ID)
			if ref.ID == p.ID {
				meta.ValidationErrors = append(meta.ValidationErrors, ValidationError{
					Path:    "content.procedure[" + ref.ID + "]",
					Message: "Auto-referência: um Procedure não pode referenciar a si mesmo.",
				})
			} else if len(allProcs) > 0 {
				if _, ok := allProcs[ref.ID]; !ok {
					meta.ValidationErrors = append(meta.ValidationErrors, ValidationError{
						Path:    "content.procedure[" + ref.ID + "]",
						Message: fmt.Sprintf("Referência inválida: o Procedure %q não existe neste agente.", ref.ID),
					})
				}
			}
		}
	}
	return meta
}

func collectStepRefs(steps []StructuredStep, meta *ProcedureMetadata) {
	seenTools := map[string]bool{}
	seenProcs := map[string]bool{}
	var walk func(list []StructuredStep)
	walk = func(list []StructuredStep) {
		for _, s := range list {
			if s.ToolID != "" && !seenTools[s.ToolID] {
				seenTools[s.ToolID] = true
				meta.ReferencedToolIDs = append(meta.ReferencedToolIDs, s.ToolID)
			}
			if s.ProcedureID != "" && !seenProcs[s.ProcedureID] {
				seenProcs[s.ProcedureID] = true
				meta.ReferencedProcedureIDs = append(meta.ReferencedProcedureIDs, s.ProcedureID)
			}
			for _, arm := range s.Branches {
				walk(arm.Steps)
			}
			walk(s.Fallback)
			if s.OnFailure != nil {
				walk(s.OnFailure.Fallback)
			}
		}
	}
	walk(steps)
}

type ActivatedProcedure struct {
	Name            string                         `json:"name"`
	Type            string                         `json:"type"`
	Trigger         string                         `json:"trigger"`
	Guardrails      []string                       `json:"guardrails"`
	ExpandedContent string                         `json:"expanded_content"`
	AllowedTools    map[string]bool                `json:"allowed_tools"` // nomes das tools permitidas
	ToolPolicies    map[string]ToolExecutionPolicy `json:"tool_policies,omitempty"`
}

// Expander resolve recursivamente referências inline com detecção de ciclos.
type Expander struct {
	Store *store.Store
}

func NewExpander(st *store.Store) *Expander {
	return &Expander{Store: st}
}

// ExpandProcedure gera a instrução completa do Procedure (Free-form ou Structured)
// com guardrails, referências expandidas inline e políticas de execução de ferramentas.
func (e *Expander) ExpandProcedure(ctx context.Context, orgID, agentID string, p store.ProcedureSnapshot, allProcs map[string]store.ProcedureSnapshot) (ActivatedProcedure, error) {
	allowedTools := map[string]bool{
		"encerrar_conversa": true,
		"transferir_numero": true,
	}
	visited := map[string]bool{p.ID: true}
	var toolPolicies map[string]ToolExecutionPolicy

	toolsByID := map[string]store.Tool{}
	if e.Store != nil {
		if toolsList, err := e.Store.ListTools(ctx, orgID); err == nil {
			for _, t := range toolsList {
				toolsByID[t.ID] = t.Tool
				if strings.Contains(p.Content, "@"+t.Name) || strings.Contains(strings.ToLower(p.Content), strings.ToLower(t.Name)) {
					allowedTools[t.Name] = true
				}
			}
		}
	}

	var expanded string
	if p.Type == "deterministic" {
		doc, err := ParseStructured(p.Content)
		if err != nil {
			return ActivatedProcedure{}, err
		}
		expanded, toolPolicies = e.CompileStructured(ProcedureCompileContext{
			AllProcs:      allProcs,
			ToolsByID:     toolsByID,
			AllowedTools:  allowedTools,
			VisitedProcID: visited,
		}, doc)
	} else {
		var err error
		expanded, err = e.expandContent(ctx, orgID, p.Content, allProcs, visited, allowedTools, 0)
		if err != nil {
			return ActivatedProcedure{}, err
		}
	}

	var guardrailBlock strings.Builder
	if len(p.Guardrails) > 0 {
		guardrailBlock.WriteString("### REGRAS E DIRETRIZES DESTE PROCEDIMENTO:\n")
		for _, g := range p.Guardrails {
			if strings.TrimSpace(g) != "" {
				guardrailBlock.WriteString(fmt.Sprintf("- %s\n", g))
			}
		}
		guardrailBlock.WriteString("\n")
	}

	fullInstructions := guardrailBlock.String() + expanded

	pType := p.Type
	if pType == "" {
		pType = "free_form"
	}

	return ActivatedProcedure{
		Name:            p.Name,
		Type:            pType,
		Trigger:         p.Trigger,
		Guardrails:      p.Guardrails,
		ExpandedContent: fullInstructions,
		AllowedTools:    allowedTools,
		ToolPolicies:    toolPolicies,
	}, nil
}

func (e *Expander) expandContent(
	ctx context.Context,
	orgID string,
	content string,
	allProcs map[string]store.ProcedureSnapshot,
	visited map[string]bool,
	allowedTools map[string]bool,
	depth int,
) (string, error) {
	if depth > 5 {
		return content, nil // Limite de recursão
	}

	resolveToken := func(kind, id, rawMatch string) string {
		kind = normalizeKind(kind)
		switch kind {
		case "tool":
			if e.Store == nil {
				return rawMatch
			}
			t, err := e.Store.GetTool(ctx, orgID, id)
			if err == nil {
				allowedTools[t.Name] = true
				return fmt.Sprintf("[Ferramenta: `%s`]", t.Name)
			}
			return rawMatch

		case "system_tool":
			sysName := id
			if sysName == "end_call" {
				sysName = "encerrar_conversa"
			}
			allowedTools[sysName] = true
			return fmt.Sprintf("[System Tool: `%s`]", sysName)

		case "procedure":
			if visited[id] {
				return "[Procedimento já incluído]"
			}
			subProc, ok := allProcs[id]
			if !ok {
				return rawMatch
			}
			visited[id] = true
			if subProc.Type == "deterministic" {
				if doc, err := ParseStructured(subProc.Content); err == nil {
					toolsByID := map[string]store.Tool{}
					if e.Store != nil {
						if list, err := e.Store.ListTools(ctx, orgID); err == nil {
							for _, tl := range list {
								toolsByID[tl.ID] = tl.Tool
							}
						}
					}
					compiled, _ := e.CompileStructured(ProcedureCompileContext{
						AllProcs:      allProcs,
						ToolsByID:     toolsByID,
						AllowedTools:  allowedTools,
						VisitedProcID: visited,
					}, doc)
					return fmt.Sprintf("\n--- SUB-PROCEDURE ESTRUTURADO: %s ---\n%s\n--- FIM DO SUB-PROCEDURE %s ---\n", subProc.Name, compiled, subProc.Name)
				}
			}
			subContent, err := e.expandContent(ctx, orgID, subProc.Content, allProcs, visited, allowedTools, depth+1)
			if err != nil {
				return rawMatch
			}
			return fmt.Sprintf("\n--- PROCEDIMENTO INCLUÍDO: %s ---\n%s\n--- FIM DO PROCEDIMENTO %s ---\n", subProc.Name, subContent, subProc.Name)

		case "knowledge":
			if e.Store == nil {
				return rawMatch
			}
			kb, err := e.Store.GetKnowledgeBase(ctx, orgID, id)
			if err != nil {
				return rawMatch
			}
			text, err := e.Store.GetKnowledgeBaseFullText(ctx, orgID, id)
			if err != nil {
				return rawMatch
			}
			return fmt.Sprintf("\n--- BASE DE CONHECIMENTO: %s ---\n%s\n--- FIM DA BASE %s ---\n", kb.Name, text, kb.Name)

		default:
			return rawMatch
		}
	}

	expanded := refTokenRe.ReplaceAllStringFunc(content, func(match string) string {
		parts := refTokenRe.FindStringSubmatch(match)
		return resolveToken(parts[1], parts[2], match)
	})

	expanded = elevenRefTokenRe.ReplaceAllStringFunc(expanded, func(match string) string {
		parts := elevenRefTokenRe.FindStringSubmatch(match)
		return resolveToken(parts[1], parts[2], match)
	})

	return expanded, nil
}
