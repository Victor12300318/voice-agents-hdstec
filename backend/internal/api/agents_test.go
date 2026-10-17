package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type detailResp struct {
	store_Agent
	LatestVersion         *int     `json:"latest_version"`
	HasUnpublishedChanges bool     `json:"has_unpublished_changes"`
	PublishErrors         []string `json:"publish_errors"`
}

type store_Agent struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Voice    string `json:"voice"`
}

func TestDraftPublishFlow(t *testing.T) {
	ts, st := newTestServer(t)
	suffix := randSuffix()
	createOrg(t, st, "Org P", "p-"+suffix+"@test.local", "senha-p-123")
	createOrg(t, st, "Org Q", "q-"+suffix+"@test.local", "senha-q-123")
	c := newClient(t, ts.URL)
	c.do("POST", "/api/auth/login", map[string]string{"email": "p-" + suffix + "@test.local", "password": "senha-p-123"}, nil)

	var created detailResp
	if code := c.do("POST", "/api/agents", map[string]string{"name": "Livia"}, &created); code != http.StatusCreated {
		t.Fatalf("criar: %d", code)
	}
	if created.Provider != "fake" || created.Model != "m1" || created.Voice != "v1" {
		t.Fatalf("padrões do provider não aplicados: %+v", created)
	}
	id := created.ID

	// Sem System Prompt: Publish bloqueado.
	var d detailResp
	c.do("GET", "/api/agents/"+id, nil, &d)
	if len(d.PublishErrors) == 0 || !d.HasUnpublishedChanges || d.LatestVersion != nil {
		t.Fatalf("estado inicial inesperado: %+v", d)
	}
	if code := c.do("POST", "/api/agents/"+id+"/publish", nil, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("publish inválido: esperado 422, veio %d", code)
	}

	// Webhook de teste que devolve a Dynamic Variable declarada.
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"dynamic_variables":{"nome_cliente":"João"}}`))
	}))
	defer hook.Close()

	update := map[string]any{
		"name": "Livia", "description": "", "active": true, "provider": "fake", "model": "m1", "voice": "v1",
		"language": "pt-BR", "timezone": "America/Sao_Paulo", "system_prompt": "Você atende {{nome_cliente}}.",
		"first_message": "{{horario}}, sou a Livia.", "first_message_interruptible": false,
		"max_duration_seconds": 300, "recording_enabled": true,
		"variables":              []map[string]string{{"name": "nome_cliente", "description": "Nome"}},
		"initiation_webhook_url": hook.URL, "initiation_webhook_timeout_ms": 2000,
		"system_tools": []string{"encerrar_conversa"},
	}
	if code := c.do("PUT", "/api/agents/"+id, update, &d); code != http.StatusOK {
		t.Fatalf("update: %d", code)
	}
	if len(d.PublishErrors) != 0 {
		t.Fatalf("não deveria haver erros de publish: %v", d.PublishErrors)
	}

	var hookResp struct {
		DynamicVariables map[string]string `json:"dynamic_variables"`
	}
	if code := c.do("POST", "/api/agents/"+id+"/initiation-webhook/test", map[string]any{"caller": "123"}, &hookResp); code != http.StatusOK || hookResp.DynamicVariables["nome_cliente"] != "João" {
		t.Fatalf("teste do webhook: %d %+v", code, hookResp)
	}

	var pub struct{ Version int }
	if code := c.do("POST", "/api/agents/"+id+"/publish", nil, &pub); code != http.StatusCreated || pub.Version != 1 {
		t.Fatalf("publish v1: %d %+v", code, pub)
	}
	c.do("GET", "/api/agents/"+id, nil, &d)
	if d.HasUnpublishedChanges || d.LatestVersion == nil || *d.LatestVersion != 1 {
		t.Fatalf("após publish: %+v", d)
	}

	// Editar o Draft marca alterações não publicadas; novo Publish gera v2.
	update["model"] = "m2"
	c.do("PUT", "/api/agents/"+id, update, &d)
	if !d.HasUnpublishedChanges {
		t.Fatal("edição deveria marcar alterações não publicadas")
	}
	c.do("POST", "/api/agents/"+id+"/publish", nil, &pub)
	if pub.Version != 2 {
		t.Fatalf("esperava v2, veio %d", pub.Version)
	}

	// Validação ao salvar.
	update["variables"] = []map[string]string{{"name": "horario"}}
	if code := c.do("PUT", "/api/agents/"+id, update, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("variable reservada: esperado 422, veio %d", code)
	}

	// Outra Organization não enxerga nem publica.
	q := newClient(t, ts.URL)
	q.do("POST", "/api/auth/login", map[string]string{"email": "q-" + suffix + "@test.local", "password": "senha-q-123"}, nil)
	if code := q.do("POST", "/api/agents/"+id+"/publish", nil, nil); code != http.StatusNotFound {
		t.Fatalf("publish de outra org: esperado 404, veio %d", code)
	}
	if code := q.do("PUT", "/api/agents/"+id, update, nil); code != http.StatusNotFound {
		t.Fatalf("update de outra org: esperado 404, veio %d", code)
	}
}
