package voice

import (
	"regexp"
	"testing"
)

var providerIDRe = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)

// VerifyCatalogCompliance valida se o catálogo de um provider atende aos requisitos do contrato.
func VerifyCatalogCompliance(t *testing.T, cat Catalog) {
	t.Helper()

	if !providerIDRe.MatchString(cat.Provider) {
		t.Fatalf("Provider ID inválido %q (deve ter 3-32 caracteres a-z, 0-9 ou _)", cat.Provider)
	}
	if cat.Label == "" {
		t.Fatal("Label do provider não pode ser vazio")
	}
	if len(cat.Models) == 0 {
		t.Fatal("o provider deve listar pelo menos um modelo")
	}
	if len(cat.Voices) == 0 {
		t.Fatal("o provider deve listar pelo menos uma voz")
	}
	if len(cat.Languages) == 0 {
		t.Fatal("o provider deve listar pelo menos um idioma")
	}
	if cat.InputSampleRate <= 0 || cat.OutputSampleRate <= 0 {
		t.Fatalf("taxas de amostragem inválidas: in=%d out=%d", cat.InputSampleRate, cat.OutputSampleRate)
	}
	if cat.MaxSessionSeconds < 30 {
		t.Fatalf("MaxSessionSeconds muito baixo: %d", cat.MaxSessionSeconds)
	}

	var hasDefModel, hasDefVoice, hasDefLang bool
	for _, m := range cat.Models {
		if m.Default {
			hasDefModel = true
			if !cat.HasModel(m.ID) {
				t.Fatalf("HasModel falhou para modelo default %q", m.ID)
			}
		}
	}
	for _, v := range cat.Voices {
		if v.Default {
			hasDefVoice = true
			if !cat.HasVoice(v.ID) {
				t.Fatalf("HasVoice falhou para voz default %q", v.ID)
			}
		}
	}
	for _, l := range cat.Languages {
		if l.Default {
			hasDefLang = true
			if !cat.HasLanguage(l.ID) {
				t.Fatalf("HasLanguage falhou para idioma default %q", l.ID)
			}
		}
	}

	if !hasDefModel {
		t.Fatal("nenhum modelo marcado como default")
	}
	if !hasDefVoice {
		t.Fatal("nenhuma voz marcada como default")
	}
	if !hasDefLang {
		t.Fatal("nenhum idioma marcado como default")
	}

	if cat.HasModel("modelo_inexistente_xyz_123") {
		t.Fatal("HasModel retornou true para modelo inexistente")
	}
	if cat.HasVoice("voz_inexistente_xyz_123") {
		t.Fatal("HasVoice retornou true para voz inexistente")
	}
	if cat.HasLanguage("idioma_inexistente_xyz_123") {
		t.Fatal("HasLanguage retornou true para idioma inexistente")
	}
}
