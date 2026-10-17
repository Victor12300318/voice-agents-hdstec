package procedures

import (
	"fmt"
	"regexp"
	"strings"
)

type ImportedSOPProcedure struct {
	Name    string `json:"name"`
	Trigger string `json:"trigger"`
	Content string `json:"content"`
}

var headingRe = regexp.MustCompile(`(?m)^(?:#{1,3}\s+|(?:PROCEDIMENTO|SOP|FLUXO|ETAPA)\s*\d*[:\-–]\s*)(.+)$`)

// ParseSOPDocument analisa um documento SOP (Markdown ou Texto) e extrai até 10 Procedures distintos
// com Name, Trigger e Content gerados automaticamente (estilo "From SOP" da ElevenLabs).
func ParseSOPDocument(docText string) []ImportedSOPProcedure {
	docText = strings.ReplaceAll(docText, "\r\n", "\n")
	docText = strings.TrimSpace(docText)
	if docText == "" {
		return nil
	}

	locs := headingRe.FindAllStringSubmatchIndex(docText, -1)
	var out []ImportedSOPProcedure

	if len(locs) == 0 {
		// Documento único sem cabeçalhos explícitos
		lines := strings.Split(docText, "\n")
		title := strings.TrimSpace(lines[0])
		if len(title) > 60 {
			title = title[:60] + "..."
		}
		if title == "" {
			title = "Procedimento Importado"
		}
		body := docText
		if len(lines) > 1 {
			body = strings.TrimSpace(strings.Join(lines[1:], "\n"))
		}
		if body == "" {
			body = docText
		}
		out = append(out, ImportedSOPProcedure{
			Name:    cleanTitle(title),
			Trigger: generateTriggerFromTitle(cleanTitle(title), body),
			Content: body,
		})
		return out
	}

	for i, loc := range locs {
		if len(out) >= 10 {
			break // Limite de até 10 procedures por importação (igual à ElevenLabs)
		}
		title := strings.TrimSpace(docText[loc[2]:loc[3]])
		bodyStart := loc[1]
		bodyEnd := len(docText)
		if i+1 < len(locs) {
			bodyEnd = locs[i+1][0]
		}
		body := strings.TrimSpace(docText[bodyStart:bodyEnd])
		if body == "" {
			continue
		}

		cTitle := cleanTitle(title)
		trigger, cleanedBody := extractExplicitTrigger(cTitle, body)
		out = append(out, ImportedSOPProcedure{
			Name:    cTitle,
			Trigger: trigger,
			Content: cleanedBody,
		})
	}

	return out
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(strings.TrimLeft(s, "#-*0123456789.) "))
	if s == "" {
		return "Procedimento SOP"
	}
	return s
}

func extractExplicitTrigger(title, body string) (string, string) {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		for _, prefix := range []string{"trigger:", "gatilho:", "quando usar:", "when:"} {
			if strings.HasPrefix(lower, prefix) {
				trig := strings.TrimSpace(trimmed[len(prefix):])
				trig = strings.Trim(trig, `"'*`)
				remaining := append(lines[:i], lines[i+1:]...)
				return trig, strings.TrimSpace(strings.Join(remaining, "\n"))
			}
		}
	}
	return generateTriggerFromTitle(title, body), body
}

func generateTriggerFromTitle(title, _ string) string {
	lower := strings.ToLower(title)
	if strings.HasPrefix(lower, "quando ") || strings.HasPrefix(lower, "when ") {
		return title
	}
	return fmt.Sprintf("Quando o cliente solicitar atendimento ou tiver dúvidas sobre %s", strings.ToLower(title))
}
