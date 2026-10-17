package procedures

import (
	"encoding/json"
	"fmt"
	"strings"

	"voiceagent/internal/store"
)

// StructuredDocument representa o JSON de um Structured Procedure (type: "deterministic" na ElevenLabs).
type StructuredDocument struct {
	Steps []StructuredStep `json:"steps"`
}

type StructuredStep struct {
	Type                string                       `json:"type"` // ask | tell | say | branch | tool_call | retry | sub_procedure | system_tool
	Instruction         string                       `json:"instruction,omitempty"`
	Message             string                       `json:"message,omitempty"`
	MessageTranslations map[string]TranslationValue  `json:"message_translations,omitempty"`
	ToolID              string                       `json:"tool_id,omitempty"`
	ToolName            string                       `json:"tool_name,omitempty"`
	SchemaOverrides     map[string]SchemaOverride    `json:"schema_overrides,omitempty"`
	OnFailure           *FailureHandler              `json:"on_failure,omitempty"`
	Branches            []ConditionalArm             `json:"branches,omitempty"`
	Fallback            []StructuredStep             `json:"fallback,omitempty"`
	ProcedureID         string                       `json:"procedure_id,omitempty"`
	SystemToolName      string                       `json:"system_tool_name,omitempty"`
	MaxRetries          int                          `json:"max_retries,omitempty"`
}

type TranslationValue struct {
	Value string `json:"value"`
}

type SchemaOverride struct {
	Source          string `json:"source"` // constant | dynamic_variable | llm | omit
	ConstantValue   any    `json:"constant_value,omitempty"`
	DynamicVariable string `json:"dynamic_variable,omitempty"`
	Prompt          string `json:"prompt,omitempty"`
}

type FailureHandler struct {
	Branches []ConditionalArm `json:"branches,omitempty"`
	Fallback []StructuredStep `json:"fallback"`
}

type ConditionalArm struct {
	Condition StepCondition    `json:"condition"`
	Steps     []StructuredStep `json:"steps"`
}

type StepCondition struct {
	Type       string          `json:"type"` // llm | expression
	Condition  string          `json:"condition,omitempty"`
	Expression *ExprCondition  `json:"expression,omitempty"`
}

type ExprCondition struct {
	Type  string      `json:"type"` // eq_operator | neq_operator | contains
	Left  ExprOperand `json:"left"`
	Right ExprOperand `json:"right"`
}

type ExprOperand struct {
	Type  string `json:"type"` // dynamic_variable | string_literal
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

type ValidationError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ToolExecutionPolicy armazena overrides e política de retry compilados de um step tool_call.
type ToolExecutionPolicy struct {
	ToolName        string                    `json:"tool_name"`
	SchemaOverrides map[string]SchemaOverride `json:"schema_overrides,omitempty"`
	MaxRetries      int                       `json:"max_retries,omitempty"`
}

// ParseStructured decodifica o JSON de um Structured Procedure.
func ParseStructured(content string) (StructuredDocument, error) {
	var doc StructuredDocument
	if strings.TrimSpace(content) == "" {
		return doc, fmt.Errorf("documento estruturado vazio")
	}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return doc, fmt.Errorf("JSON de Structured Procedure inválido: %w", err)
	}
	return doc, nil
}

// ValidateStructured aplica todas as regras de validação de Structured Procedures da ElevenLabs.
func ValidateStructured(
	selfID string,
	content string,
	allProcs map[string]store.ProcedureSnapshot,
	toolsByID map[string]store.Tool,
) []ValidationError {
	doc, err := ParseStructured(content)
	if err != nil {
		return []ValidationError{{Path: "content", Message: err.Error()}}
	}
	if len(doc.Steps) == 0 {
		return []ValidationError{{Path: "steps", Message: "A lista de steps não pode estar vazia."}}
	}

	var errs []ValidationError
	validateStepList(doc.Steps, "steps", selfID, false, false, allProcs, toolsByID, &errs)
	return errs
}

func validateStepList(
	steps []StructuredStep,
	basePath string,
	selfID string,
	insideIf bool,
	insideFailure bool,
	allProcs map[string]store.ProcedureSnapshot,
	toolsByID map[string]store.Tool,
	errs *[]ValidationError,
) {
	addErr := func(path, msg string) {
		*errs = append(*errs, ValidationError{Path: path, Message: msg})
	}

	for i, st := range steps {
		stepPath := fmt.Sprintf("%s[%d]", basePath, i)

		// Regra: Dois If (branch) não podem ficar seguidos (back to back)
		if i > 0 && st.Type == "branch" && steps[i-1].Type == "branch" {
			addErr(stepPath+".branch", fmt.Sprintf("Step %d: Dois blocos If (branch) não podem ficar em sequência direta.", i+1))
		}

		// Regra: If com condição expression não pode vir imediatamente após um Ask
		if i > 0 && st.Type == "branch" && steps[i-1].Type == "ask" && len(st.Branches) > 0 && st.Branches[0].Condition.Type == "expression" {
			addErr(stepPath+".branch", fmt.Sprintf("Step %d: Um bloco If com condição de variável (expression) não pode vir diretamente após um Ask.", i+1))
		}

		switch st.Type {
		case "ask":
			if strings.TrimSpace(st.Instruction) == "" {
				addErr(stepPath+".ask.instruction", fmt.Sprintf("Step %d: O bloco Ask exige uma instrução.", i+1))
			}

		case "tell":
			if strings.TrimSpace(st.Instruction) == "" {
				addErr(stepPath+".tell.instruction", fmt.Sprintf("Step %d: O bloco Tell exige uma instrução.", i+1))
			}

		case "say":
			if strings.TrimSpace(st.Message) == "" {
				addErr(stepPath+".say.message", fmt.Sprintf("Step %d: O bloco Say exige uma mensagem exata.", i+1))
			}

		case "tool_call":
			if insideFailure {
				addErr(stepPath+".tool_call", fmt.Sprintf("Step %d: Não é permitido chamar outra Tool dentro de um tratador de falha (on_failure).", i+1))
			}
			if strings.TrimSpace(st.ToolID) == "" && strings.TrimSpace(st.ToolName) == "" {
				addErr(stepPath+".tool_call.tool_id", fmt.Sprintf("Step %d: O bloco Tool exige selecionar uma ferramenta.", i+1))
			} else if st.ToolID != "" && len(toolsByID) > 0 {
				if _, ok := toolsByID[st.ToolID]; !ok {
					addErr(stepPath+".tool_call.tool_id", fmt.Sprintf("Step %d: A ferramenta referenciada (%s) não existe ou foi removida.", i+1, st.ToolID))
				}
			}
			if st.OnFailure != nil {
				if len(st.OnFailure.Fallback) == 0 {
					addErr(stepPath+".tool_call.on_failure.fallback", fmt.Sprintf("Step %d: O tratador de falha (on_failure) deve conter pelo menos um passo.", i+1))
				} else {
					validateStepList(st.OnFailure.Fallback, stepPath+".on_failure.fallback", selfID, insideIf, true, allProcs, toolsByID, errs)
				}
			}

		case "branch":
			if insideIf {
				addErr(stepPath+".branch", fmt.Sprintf("Step %d: Blocos If (branch) não podem ser aninhados. Extraia a decisão interna para um Sub-procedure.", i+1))
			}
			if insideFailure {
				addErr(stepPath+".branch", fmt.Sprintf("Step %d: Blocos If (branch) não são permitidos dentro de on_failure.", i+1))
			}
			if len(st.Branches) == 0 {
				addErr(stepPath+".branch.branches", fmt.Sprintf("Step %d: O bloco If precisa de pelo menos uma condição.", i+1))
			}
			var condType string
			for bIdx, arm := range st.Branches {
				armPath := fmt.Sprintf("%s.branches[%d]", stepPath, bIdx)
				cType := arm.Condition.Type
				if cType == "" {
					cType = "llm"
				}
				if condType == "" {
					condType = cType
				} else if condType != cType {
					addErr(armPath+".condition.type", fmt.Sprintf("Step %d: Todos os ramos de um mesmo bloco If devem usar o mesmo tipo de condição (%s).", i+1, condType))
				}
				if cType == "llm" && strings.TrimSpace(arm.Condition.Condition) == "" {
					addErr(armPath+".condition", fmt.Sprintf("Step %d (Ramo %d): A condição em linguagem natural não pode estar vazia.", i+1, bIdx+1))
				}
				if len(arm.Steps) == 0 {
					addErr(armPath+".steps", fmt.Sprintf("Step %d (Ramo %d): O ramo condicional deve conter pelo menos um passo.", i+1, bIdx+1))
				} else {
					validateStepList(arm.Steps, armPath+".steps", selfID, true, insideFailure, allProcs, toolsByID, errs)
				}
			}
			if len(st.Fallback) > 0 {
				validateStepList(st.Fallback, stepPath+".fallback", selfID, true, insideFailure, allProcs, toolsByID, errs)
			}

		case "retry":
			if !insideFailure {
				addErr(stepPath+".retry", fmt.Sprintf("Step %d: O bloco Retry só é válido dentro do tratador de falha (on_failure) de uma Tool.", i+1))
			}
			if i != len(steps)-1 {
				addErr(stepPath+".retry", fmt.Sprintf("Step %d: O bloco Retry deve ser obrigatoriamente o último passo do tratador de falha.", i+1))
			}
			if st.MaxRetries < 0 || st.MaxRetries > 3 {
				addErr(stepPath+".retry.max_retries", fmt.Sprintf("Step %d: max_retries deve ser entre 1 e 3.", i+1))
			}

		case "sub_procedure":
			if strings.TrimSpace(st.ProcedureID) == "" {
				addErr(stepPath+".sub_procedure.procedure_id", fmt.Sprintf("Step %d: Selecione o Sub-procedure de destino.", i+1))
			} else if st.ProcedureID == selfID && selfID != "" {
				addErr(stepPath+".sub_procedure.procedure_id", fmt.Sprintf("Step %d: Um Procedure não pode invocar a si mesmo.", i+1))
			} else if len(allProcs) > 0 {
				if _, ok := allProcs[st.ProcedureID]; !ok {
					addErr(stepPath+".sub_procedure.procedure_id", fmt.Sprintf("Step %d: O Sub-procedure referenciado (%s) não existe neste agente.", i+1, st.ProcedureID))
				}
			}

		case "system_tool":
			if st.SystemToolName == "" {
				addErr(stepPath+".system_tool.system_tool_name", fmt.Sprintf("Step %d: Informe a System Tool (ex: end_call / encerrar_conversa).", i+1))
			}
			if (st.SystemToolName == "end_call" || st.SystemToolName == "encerrar_conversa") && i != len(steps)-1 {
				addErr(stepPath+".system_tool", fmt.Sprintf("Step %d: O encerramento da chamada (%s) é terminal e deve ser o último passo da sequência.", i+1, st.SystemToolName))
			}

		default:
			addErr(stepPath+".type", fmt.Sprintf("Step %d: Tipo de passo desconhecido %q.", i+1, st.Type))
		}
	}
}

// CompileStructured compila os steps de um Structured Procedure em instruções determinísticas
// para o Voice Provider e extrai as ferramentas liberadas e políticas de execução.
func (e *Expander) CompileStructured(
	ctx ProcedureCompileContext,
	doc StructuredDocument,
) (string, map[string]ToolExecutionPolicy) {
	policies := make(map[string]ToolExecutionPolicy)
	var sb strings.Builder
	sb.WriteString("### PROCEDIMENTO ESTRUTURADO DETERMINÍSTICO (EXECUTE OS PASSOS RIGOROSAMENTE NA ORDEM ABAIXO):\n")
	sb.WriteString("Regras de execução:\n")
	sb.WriteString("- Apenas passos [ASK] aguardam resposta do Caller. Passos [TELL] e [SAY] devem ser falados e seguidos imediatamente pelo próximo passo no mesmo turno.\n")
	sb.WriteString("- Em passos [SAY], fale a mensagem EXATAMENTE palavra por palavra como escrita.\n\n")

	compileStepSequence(&sb, doc.Steps, "", ctx, policies, 0)
	return sb.String(), policies
}

type ProcedureCompileContext struct {
	AllProcs      map[string]store.ProcedureSnapshot
	ToolsByID     map[string]store.Tool
	AllowedTools  map[string]bool
	Language      string
	VisitedProcID map[string]bool
}

func compileStepSequence(
	sb *strings.Builder,
	steps []StructuredStep,
	indent string,
	ctx ProcedureCompileContext,
	policies map[string]ToolExecutionPolicy,
	depth int,
) {
	if depth > 5 {
		return
	}
	for i, st := range steps {
		num := fmt.Sprintf("%s%d.", indent, i+1)
		switch st.Type {
		case "ask":
			sb.WriteString(fmt.Sprintf("%s **[ASK — Pergunte e aguarde a resposta do Caller]**: %s\n", num, st.Instruction))

		case "tell":
			sb.WriteString(fmt.Sprintf("%s **[TELL — Informe com suas palavras e avance imediatamente]**: %s\n", num, st.Instruction))

		case "say":
			msg := st.Message
			if ctx.Language != "" && st.MessageTranslations != nil {
				if tr, ok := st.MessageTranslations[ctx.Language]; ok && tr.Value != "" {
					msg = tr.Value
				}
			}
			sb.WriteString(fmt.Sprintf("%s **[SAY — Fale literalmente palavra por palavra]**: %q\n", num, msg))

		case "tool_call":
			toolName := st.ToolName
			if st.ToolID != "" {
				if t, ok := ctx.ToolsByID[st.ToolID]; ok {
					toolName = t.Name
				}
			}
			if toolName != "" {
				ctx.AllowedTools[toolName] = true
			}
			instr := st.Instruction
			if instr == "" {
				instr = "Execute a ferramenta com os dados coletados."
			}
			sb.WriteString(fmt.Sprintf("%s **[TOOL CALL — Chame obrigatoriamente a ferramenta `%s`]**: %s\n", num, toolName, instr))

			maxRetries := 0
			if st.OnFailure != nil {
				for _, fbStep := range st.OnFailure.Fallback {
					if fbStep.Type == "retry" {
						if fbStep.MaxRetries > 0 {
							maxRetries = fbStep.MaxRetries
						} else {
							maxRetries = 1
						}
					}
				}
				sb.WriteString(fmt.Sprintf("%s   ↳ *Se a ferramenta `%s` falhar, execute este tratamento de erro:*\n", indent, toolName))
				compileStepSequence(sb, st.OnFailure.Fallback, indent+"     ", ctx, policies, depth+1)
			}

			if toolName != "" {
				policies[toolName] = ToolExecutionPolicy{
					ToolName:        toolName,
					SchemaOverrides: st.SchemaOverrides,
					MaxRetries:      maxRetries,
				}
			}

		case "branch":
			sb.WriteString(fmt.Sprintf("%s **[IF / CONDICIONAL — Avalie na ordem e execute apenas o primeiro ramo verdadeiro]**:\n", num))
			for bIdx, arm := range st.Branches {
				label := "IF"
				if bIdx > 0 {
					label = "ELSE IF"
				}
				condDesc := arm.Condition.Condition
				if arm.Condition.Type == "expression" && arm.Condition.Expression != nil {
					ex := arm.Condition.Expression
					condDesc = fmt.Sprintf("variável {{%s}} == %q", ex.Left.Name, ex.Right.Value)
				}
				sb.WriteString(fmt.Sprintf("%s   - **%s (%s)**:\n", indent, label, condDesc))
				compileStepSequence(sb, arm.Steps, indent+"     ", ctx, policies, depth+1)
			}
			if len(st.Fallback) > 0 {
				sb.WriteString(fmt.Sprintf("%s   - **ELSE (Caso nenhuma condição acima seja atendida)**:\n", indent))
				compileStepSequence(sb, st.Fallback, indent+"     ", ctx, policies, depth+1)
			}

		case "retry":
			retries := st.MaxRetries
			if retries <= 0 {
				retries = 1
			}
			sb.WriteString(fmt.Sprintf("%s **[RETRY]**: Tente executar a ferramenta novamente (até %d tentativa(s) extra).\n", num, retries))

		case "sub_procedure":
			sub, ok := ctx.AllProcs[st.ProcedureID]
			if !ok {
				sb.WriteString(fmt.Sprintf("%s **[SUB-PROCEDURE `%s`]**\n", num, st.ProcedureID))
				continue
			}
			if ctx.VisitedProcID[sub.ID] {
				continue
			}
			ctx.VisitedProcID[sub.ID] = true
			sb.WriteString(fmt.Sprintf("%s **[SUB-PROCEDURE: %s — Execute os passos abaixo e depois retorne para o próximo passo]**:\n", num, sub.Name))
			if sub.Type == "deterministic" {
				if subDoc, err := ParseStructured(sub.Content); err == nil {
					compileStepSequence(sb, subDoc.Steps, indent+"   ", ctx, policies, depth+1)
				}
			} else {
				sb.WriteString(indent + "   " + sub.Content + "\n")
			}

		case "system_tool":
			sysName := st.SystemToolName
			if sysName == "end_call" {
				sysName = "encerrar_conversa"
			}
			ctx.AllowedTools[sysName] = true
			sb.WriteString(fmt.Sprintf("%s **[SYSTEM TOOL — Chame `%s` agora]**\n", num, sysName))
		}
	}
}
