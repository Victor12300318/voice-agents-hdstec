package store

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestNormalizeE164(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"+55 11 99999-8888", "+5511999998888"},
		{"5511999998888", "+5511999998888"},
		{"+1 (800) 555-0199", "+18005550199"},
		{"  +55 (21) 98765-4321  ", "+5521987654321"},
		{"", ""},
	}

	for _, c := range cases {
		got := normalizeE164(c.input)
		if got != c.expected {
			t.Errorf("normalizeE164(%q) = %q; expected %q", c.input, got, c.expected)
		}
	}
}

func TestPhoneNumberAgentAssignmentAndNullability(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL / DATABASE_URL not set; skipping store integration test")
	}

	ctx := context.Background()
	st, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	if err := st.Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	orgID, _, err := st.EnsureBootstrapAdmin(ctx, "Org Phone Test", "phone-test-admin@test.local", "hash-123")
	if err != nil {
		t.Fatalf("bootstrap org: %v", err)
	}

	agA, err := st.CreateAgent(ctx, orgID, "Agente A", "", "gemini", "gemini-2.5-flash", "Kore")
	if err != nil {
		t.Fatalf("create agent A: %v", err)
	}
	agB, err := st.CreateAgent(ctx, orgID, "Agente B", "", "gemini", "gemini-2.5-flash", "Puck")
	if err != nil {
		t.Fatalf("create agent B: %v", err)
	}

	// 1. Cria número sem Agent vinculado (AgentID vazio -> NULL no banco)
	created, err := st.CreatePhoneNumber(ctx, orgID, CreatePhoneNumberParams{
		E164:             "+55 11 4097-8278",
		Label:            "Twilio Ricardo",
		AgentID:          "",
		Direction:        "both",
		Provider:         "twilio",
		TwilioAccountSid: "AC1234567890",
		TwilioAuthToken:  "secret-token",
		TwilioPhoneSid:   "PN1234567890",
	})
	if err != nil {
		t.Fatalf("create unassigned phone number: %v", err)
	}
	if created.AgentID != "" || created.AgentName != "" {
		t.Fatalf("expected empty agent_id and agent_name, got %q / %q", created.AgentID, created.AgentName)
	}
	if !created.Active || created.Provider != "twilio" || created.Direction != "both" || created.Label != "Twilio Ricardo" {
		t.Fatalf("unexpected created fields: %+v", created)
	}

	// 2. FindAgentByPhoneNumber retorna ErrPhoneNumberUnassigned quando sem agente
	if _, _, err := st.FindAgentByPhoneNumber(ctx, "+551140978278"); !errors.Is(err, ErrPhoneNumberUnassigned) {
		t.Fatalf("expected ErrPhoneNumberUnassigned, got %v", err)
	}

	// 3. Atualiza parcialmente apenas o AgentID para Agente A, preservando Label, Provider, Direction, Active e Twilio
	aidA := agA.ID
	updatedA, err := st.UpdatePhoneNumber(ctx, orgID, created.ID, UpdatePhoneNumberParams{
		AgentID: &aidA,
	})
	if err != nil {
		t.Fatalf("update phone agent to A: %v", err)
	}
	if updatedA.AgentID != agA.ID || updatedA.AgentName != "Agente A" {
		t.Fatalf("expected agent A (%s), got %q (%q)", agA.ID, updatedA.AgentID, updatedA.AgentName)
	}
	if !updatedA.Active || updatedA.Provider != "twilio" || updatedA.Direction != "both" || updatedA.Label != "Twilio Ricardo" || !updatedA.HasTwilioAuth {
		t.Fatalf("partial update corrupted existing fields: %+v", updatedA)
	}

	// 4. Troca de Agente A para Agente B
	aidB := agB.ID
	updatedB, err := st.UpdatePhoneNumber(ctx, orgID, created.ID, UpdatePhoneNumberParams{
		AgentID: &aidB,
	})
	if err != nil {
		t.Fatalf("update phone agent to B: %v", err)
	}
	if updatedB.AgentID != agB.ID || updatedB.AgentName != "Agente B" {
		t.Fatalf("expected agent B (%s), got %q (%q)", agB.ID, updatedB.AgentID, updatedB.AgentName)
	}

	// 5. Desvincula o Agente (AgentID = "")
	empty := ""
	unlinked, err := st.UpdatePhoneNumber(ctx, orgID, created.ID, UpdatePhoneNumberParams{
		AgentID: &empty,
	})
	if err != nil {
		t.Fatalf("unlink phone agent: %v", err)
	}
	if unlinked.AgentID != "" || unlinked.AgentName != "" || !unlinked.Active {
		t.Fatalf("expected unlinked active phone number, got %+v", unlinked)
	}

	// 6. Vincula novamente ao Agente B e verifica ON DELETE SET NULL ao excluir o Agente B
	if _, err := st.UpdatePhoneNumber(ctx, orgID, created.ID, UpdatePhoneNumberParams{AgentID: &aidB}); err != nil {
		t.Fatalf("relink agent B: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `DELETE FROM agents WHERE id = $1`, agB.ID); err != nil {
		t.Fatalf("delete agent B: %v", err)
	}
	afterDelete, err := st.GetPhoneNumberWithAgent(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("phone number should survive agent deletion: %v", err)
	}
	if afterDelete.AgentID != "" || afterDelete.AgentName != "" {
		t.Fatalf("expected ON DELETE SET NULL to clear agent_id, got %+v", afterDelete)
	}
}
