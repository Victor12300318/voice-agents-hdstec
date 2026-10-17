package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"voiceagent/internal/store"
)

func TestExecutorSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Errorf("header ausente ou errado: %s", r.Header.Get("Authorization"))
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["cpf"] != "123" {
			t.Errorf("corpo inválido: %+v", req)
		}

		w.Header().Set("Content-Type", "application/json")
		// Endpoint do n8n devolve dados sem envelope
		w.Write([]byte(`{"nome":"João","statusPlano":"Ativo"}`))
	}))
	defer srv.Close()

	tool := store.Tool{
		Name:      "consultar",
		Config:    store.WebhookConfig{URL: srv.URL, Method: "POST"},
		SecretHeaders: map[string]string{
			"Authorization": "Bearer secret-token",
		},
		TimeoutMS: 2000,
	}

	exec := NewExecutor(http.DefaultClient)
	res := exec.Execute(context.Background(), tool, map[string]any{"cpf": "123"})

	if res.Status != "success" {
		t.Fatalf("esperava success, veio: %s (%s)", res.Status, res.ErrorMessage)
	}
	if res.Result["success"] != true {
		t.Fatalf("envelope incorreto: %+v", res.Result)
	}
	data, ok := res.Result["data"].(map[string]any)
	if !ok || data["nome"] != "João" {
		t.Fatalf("dados não normalizados: %+v", res.Result)
	}
}

func TestExecutorErrors(t *testing.T) {
	cases := map[string]struct {
		handler func(w http.ResponseWriter, r *http.Request)
		timeout int
	}{
		"HTTP 500": {
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500); w.Write([]byte("internal err")) },
			timeout: 2000,
		},
		"JSON inválido": {
			handler: func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>bad gateway</html>")) },
			timeout: 2000,
		},
		"timeout": {
			handler: func(w http.ResponseWriter, r *http.Request) { time.Sleep(300 * time.Millisecond) },
			timeout: 50,
		},
	}

	exec := NewExecutor(http.DefaultClient)
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(tc.handler))
			defer srv.Close()

			tool := store.Tool{
				Config:    store.WebhookConfig{URL: srv.URL, Method: "POST"},
				TimeoutMS: tc.timeout,
			}
			res := exec.Execute(context.Background(), tool, nil)
			if res.Status != "error" {
				t.Fatalf("esperava status error, veio %s", res.Status)
			}
			if res.Result["success"] != false || res.Result["error"] == nil {
				t.Fatalf("envelope de erro incorreto: %+v", res.Result)
			}
		})
	}
}
