package telephony

import (
	"strings"
	"testing"
)

func TestBuildTwilioStreamTwiML(t *testing.T) {
	twiml := BuildTwilioStreamTwiML("wss://example.com/ws/telephony/twilio/audio", map[string]string{
		"call_sid": "CA123456",
		"did":      "+551140041234",
	})

	if !strings.Contains(twiml, `<Connect>`) {
		t.Fatalf("expected <Connect> in TwiML, got: %s", twiml)
	}
	if !strings.Contains(twiml, `url="wss://example.com/ws/telephony/twilio/audio"`) {
		t.Fatalf("expected Stream URL in TwiML, got: %s", twiml)
	}
	if !strings.Contains(twiml, `name="call_sid" value="CA123456"`) {
		t.Fatalf("expected call_sid Parameter in TwiML, got: %s", twiml)
	}
}

func TestBuildTwilioRejectTwiML(t *testing.T) {
	twiml := BuildTwilioRejectTwiML("Número indisponível")
	if !strings.Contains(twiml, `<Say language="pt-BR">Número indisponível</Say>`) {
		t.Fatalf("expected <Say> in TwiML, got: %s", twiml)
	}
	if !strings.Contains(twiml, `<Hangup></Hangup>`) && !strings.Contains(twiml, `<Hangup/>`) {
		t.Fatalf("expected <Hangup> in TwiML, got: %s", twiml)
	}
}
