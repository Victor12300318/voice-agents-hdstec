package variables

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSystemGreetingByTimezone(t *testing.T) {
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	// 13:00 UTC = 10:00 em São Paulo
	now := time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC)
	v := System(now, sp, "conv-1")
	if v["horario"] != "Bom dia" || v["hora"] != "10:00" || v["data"] != "26/09/2026" || v["dia_semana"] != "sábado" || v["id_conversa"] != "conv-1" {
		t.Fatalf("inesperado: %+v", v)
	}
	if System(now.Add(6*time.Hour), sp, "")["horario"] != "Boa tarde" {
		t.Fatal("16h deveria ser Boa tarde")
	}
	if System(now.Add(10*time.Hour), sp, "")["horario"] != "Boa noite" {
		t.Fatal("20h deveria ser Boa noite")
	}
}

func TestRenderAndReferences(t *testing.T) {
	out := Render("{{horario}}, {{ nome }}! {{desconhecida}}", map[string]string{"horario": "Bom dia", "nome": "João"})
	if out != "Bom dia, João! {{desconhecida}}" {
		t.Fatalf("render: %q", out)
	}
	refs := References("{{a}} {{b}}", "{{ a }} {{c}}")
	if strings.Join(refs, ",") != "a,b,c" {
		t.Fatalf("refs: %v", refs)
	}
}

func webhook(t *testing.T, h http.HandlerFunc) string {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestFetchDynamic(t *testing.T) {
	declared := []Declared{{Name: "nome_cliente"}, {Name: "protocolo"}}
	req := InitiationRequest{AgentID: "a1", ConversationID: "c1", Caller: "5511999999999"}

	t.Run("sucesso envia payload e converte escalares", func(t *testing.T) {
		url := webhook(t, func(w http.ResponseWriter, r *http.Request) {
			var got InitiationRequest
			json.NewDecoder(r.Body).Decode(&got)
			if got.AgentID != "a1" || got.ConversationID != "c1" || got.Caller != "5511999999999" {
				t.Errorf("payload: %+v", got)
			}
			w.Write([]byte(`{"dynamic_variables":{"nome_cliente":"João","protocolo":12345,"horario":"ignorada"}}`))
		})
		vars, err := FetchDynamic(context.Background(), http.DefaultClient, url, time.Second, req, declared)
		if err != nil {
			t.Fatal(err)
		}
		if vars["nome_cliente"] != "João" || vars["protocolo"] != "12345" {
			t.Fatalf("vars: %+v", vars)
		}
		if _, ok := vars["horario"]; ok {
			t.Fatal("System Variable não pode ser sobrescrita pelo webhook")
		}
	})

	cases := map[string]http.HandlerFunc{
		"HTTP 500":          func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
		"JSON inválido":     func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>")) },
		"Variable ausente":  func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"dynamic_variables":{"nome_cliente":"x"}}`)) },
		"timeout":           func(w http.ResponseWriter, r *http.Request) { time.Sleep(300 * time.Millisecond) },
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			url := webhook(t, h)
			_, err := FetchDynamic(context.Background(), http.DefaultClient, url, 100*time.Millisecond, req, declared)
			if !errors.Is(err, ErrInitiation) {
				t.Fatalf("esperava ErrInitiation, veio %v", err)
			}
			t.Log(err)
		})
	}
}
