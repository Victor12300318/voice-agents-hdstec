// Package runtime gerencia as Conversations ativas: conexão WebSocket com o navegador,
// sessão com o Voice Provider, variáveis, transcrições e interrupções.
package runtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"voiceagent/internal/auth"
	"voiceagent/internal/guardian"
	"voiceagent/internal/storage"
	"voiceagent/internal/store"
	"voiceagent/internal/variables"
	"voiceagent/internal/voice"
)

type initiationPrefetchEntry struct {
	done chan struct{}
	res  variables.InitiationResult
	err  error
}

type Runtime struct {
	Store        *store.Store
	Voices       *voice.Registry
	Sessions     *auth.Sessions
	Storage      *storage.Storage
	HTTPClient   *http.Client
	Guardian     *guardian.Evaluator
	Log          *slog.Logger
	prefetchMu   sync.Mutex
	prefetchByID map[string]*initiationPrefetchEntry
}

// PrefetchInitiation dispara em paralelo a busca do Initiation Webhook durante o ringing/conexão da Twilio
// (conforme a especificação Twilio Personalization da ElevenLabs), para que os dados já estejam prontos
// quando o Media Stream WebSocket abrir.
func (rt *Runtime) PrefetchInitiation(
	callSid, agentID, callerID, calledNumber string,
	cfg store.AgentConfig,
) {
	if callSid == "" || cfg.InitiationWebhookURL == "" {
		return
	}
	rt.prefetchMu.Lock()
	if rt.prefetchByID == nil {
		rt.prefetchByID = make(map[string]*initiationPrefetchEntry)
	}
	if _, exists := rt.prefetchByID[callSid]; exists {
		rt.prefetchMu.Unlock()
		return
	}
	entry := &initiationPrefetchEntry{done: make(chan struct{})}
	rt.prefetchByID[callSid] = entry
	rt.prefetchMu.Unlock()

	go func() {
		timeout := time.Duration(cfg.InitiationWebhookTimeout) * time.Millisecond
		if timeout <= 0 {
			timeout = 3 * time.Second
		}
		client := rt.HTTPClient
		if client == nil {
			client = http.DefaultClient
		}
		res, err := variables.FetchInitiationData(
			context.Background(),
			client,
			cfg.InitiationWebhookURL,
			timeout,
			variables.InitiationRequest{
				CallerID:       callerID,
				AgentID:        agentID,
				CalledNumber:   calledNumber,
				CallSid:        callSid,
				ConversationID: callSid,
				Caller:         callerID,
				Metadata:       map[string]any{},
			},
			cfg.Variables,
		)
		entry.res = res
		entry.err = err
		close(entry.done)

		// Limpa a entrada após 30s caso o WebSocket não consuma
		time.AfterFunc(30*time.Second, func() {
			rt.prefetchMu.Lock()
			delete(rt.prefetchByID, callSid)
			rt.prefetchMu.Unlock()
		})
	}()
}

func (rt *Runtime) consumePrefetchedInitiation(ctx context.Context, callSid string) (variables.InitiationResult, bool, error) {
	if callSid == "" {
		return variables.InitiationResult{}, false, nil
	}
	rt.prefetchMu.Lock()
	entry, ok := rt.prefetchByID[callSid]
	if ok {
		delete(rt.prefetchByID, callSid)
	}
	rt.prefetchMu.Unlock()
	if !ok {
		return variables.InitiationResult{}, false, nil
	}
	select {
	case <-entry.done:
		return entry.res, true, entry.err
	case <-ctx.Done():
		return variables.InitiationResult{}, true, ctx.Err()
	}
}

type clientMessage struct {
	Type string `json:"type"` // hangup
}

type ServerMessage struct {
	Type             string            `json:"type"`
	ConversationID   string            `json:"conversation_id,omitempty"`
	InputSampleRate  int               `json:"input_sample_rate,omitempty"`
	OutputSampleRate int               `json:"output_sample_rate,omitempty"`
	Text             string            `json:"text,omitempty"`
	Reason           string            `json:"reason,omitempty"`
	Message          string            `json:"message,omitempty"`
	Variables        map[string]string `json:"variables,omitempty"`
	CostUSD          float64           `json:"cost_usd,omitempty"`
	CostBRL          float64           `json:"cost_brl,omitempty"`
}

type serverMessage = ServerMessage

// ServeWS atende o WebSocket de voz do navegador:
// GET /ws/conversations/{agent_id}?version=draft|latest|<int>&caller=<string>
func (rt *Runtime) ServeWS(w http.ResponseWriter, r *http.Request) {
	if rt.Log == nil {
		rt.Log = slog.Default()
	}
	// 1. Autenticação via cookie de sessão
	c, err := r.Cookie(auth.CookieName)
	if err != nil {
		http.Error(w, "não autenticado", http.StatusUnauthorized)
		return
	}
	userSess, err := rt.Sessions.Get(r.Context(), c.Value)
	if err != nil {
		http.Error(w, "sessão inválida", http.StatusUnauthorized)
		return
	}

	agentID := r.PathValue("agent_id")
	agent, err := rt.Store.GetAgent(r.Context(), userSess.OrganizationID, agentID)
	if err != nil {
		http.Error(w, "agente não encontrado", http.StatusNotFound)
		return
	}

	// 2. Resolve versão: draft ou publicada
	versionParam := strings.TrimSpace(r.URL.Query().Get("version"))
	if versionParam == "" {
		versionParam = "draft"
	}

	var isTest bool
	var versionID *string
	var cfg store.AgentConfig

	if versionParam == "draft" {
		isTest = true
		cfg, err = rt.Store.DraftConfig(r.Context(), userSess.OrganizationID, agent)
		if err != nil {
			http.Error(w, "erro ao carregar draft", http.StatusInternalServerError)
			return
		}
	} else {
		isTest = false
		var v store.AgentVersion
		if versionParam == "latest" {
			v, err = rt.Store.LatestVersion(r.Context(), userSess.OrganizationID, agent.ID)
		} else {
			var num int
			num, err = strconv.Atoi(versionParam)
			if err == nil {
				// Busca versão específica
				v, err = rt.getSpecificVersion(r.Context(), userSess.OrganizationID, agent.ID, num)
			}
		}
		if err != nil {
			http.Error(w, "nenhuma versão publicada disponível para este agente", http.StatusUnprocessableEntity)
			return
		}
		versionID = &v.ID
		cfg = v.Snapshot
	}

	// Agente precisa estar ativo para conversas não-teste
	if !isTest && !agent.Active {
		http.Error(w, "agente inativo", http.StatusForbidden)
		return
	}

	// Validação mínima de prontidão
	if cfg.Provider == "" {
		http.Error(w, "agente não possui Voice Provider configurado", http.StatusUnprocessableEntity)
		return
	}
	prov, err := rt.Voices.Get(cfg.Provider)
	if err != nil {
		http.Error(w, "Voice Provider indisponível", http.StatusUnprocessableEntity)
		return
	}

	// 3. Aceita conexão WebSocket
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // proxy pelo nginx na mesma porta
	})
	if err != nil {
		rt.Log.Error("ws_accept_error", "error", err)
		return
	}
	defer ws.Close(websocket.StatusInternalError, "fechando")

	// Permite chunks de áudio de até 10 MB sem estourar o limite padrão de 32 KB
	ws.SetReadLimit(10 * 1024 * 1024)

	caller := r.URL.Query().Get("caller")
	rt.runSession(r.Context(), ws, userSess.OrganizationID, agent.ID, versionID, isTest, cfg, prov, caller)
}

func (rt *Runtime) getSpecificVersion(ctx context.Context, orgID, agentID string, version int) (store.AgentVersion, error) {
	var v store.AgentVersion
	var raw []byte
	err := rt.Store.Pool.QueryRow(ctx, `
		SELECT id, agent_id, version, snapshot, published_by, created_at
		FROM agent_versions
		WHERE organization_id=$1 AND agent_id=$2 AND version=$3`, orgID, agentID, version).
		Scan(&v.ID, &v.AgentID, &v.Version, &raw, &v.PublishedBy, &v.CreatedAt)
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(raw, &v.Snapshot)
}
