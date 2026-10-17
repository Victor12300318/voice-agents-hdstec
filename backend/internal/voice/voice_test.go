package voice

import (
	"context"
	"testing"
)

type dummyProvider struct{}

func (dummyProvider) Catalog() Catalog {
	return Catalog{
		Provider: "dummy_voice", Label: "Dummy Voice", MaxSessionSeconds: 300,
		Models:           []Option{{ID: "dummy-1", Label: "Dummy 1", Default: true}},
		Voices:           []Option{{ID: "dummy-voice", Label: "Dummy Voice", Default: true}},
		Languages:        []Option{{ID: "pt-BR", Label: "Português", Default: true}},
		InputSampleRate:  16000,
		OutputSampleRate: 24000,
	}
}

func (dummyProvider) Connect(ctx context.Context, cfg SessionConfig) (Session, error) {
	return nil, nil
}

func TestCatalogCompliance(t *testing.T) {
	p := dummyProvider{}
	VerifyCatalogCompliance(t, p.Catalog())
}

func TestRegistry(t *testing.T) {
	p := dummyProvider{}
	reg := NewRegistry(p)

	got, err := reg.Get("dummy_voice")
	if err != nil {
		t.Fatal(err)
	}
	if got.Catalog().Provider != "dummy_voice" {
		t.Fatalf("provider incorreto: %s", got.Catalog().Provider)
	}

	_, err = reg.Get("inexistente")
	if err != ErrUnknownProvider {
		t.Fatalf("esperava ErrUnknownProvider, veio %v", err)
	}

	cats := reg.Catalogs()
	if len(cats) != 1 || cats[0].Provider != "dummy_voice" {
		t.Fatalf("catálogos incorretos: %+v", cats)
	}
}
