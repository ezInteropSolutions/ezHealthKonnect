package assembly

import (
	"errors"
	"testing"

	mappinglog "ezhealthkonnect/services/cda_fhir/mapping_log"
)

type stubRule struct {
	name    string
	callErr error
	called  bool
}

func (r *stubRule) Name() string { return r.name }
func (r *stubRule) Apply(_ *AssemblyContext) error {
	r.called = true
	return r.callErr
}

func TestDefaultRuleEngine_RunsAllRules(t *testing.T) {
	r1 := &stubRule{name: "rule-a"}
	r2 := &stubRule{name: "rule-b"}

	engine := NewDefaultRuleEngine()
	engine.Register(r1)
	engine.Register(r2)

	ctx := &AssemblyContext{
		Resources:      nil,
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            mappinglog.NewLogBuilder("doc"),
		Config:         AssemblyConfig{},
	}

	if err := engine.Run(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !r1.called || !r2.called {
		t.Error("both rules should have been called")
	}
}

func TestDefaultRuleEngine_ContinuesAfterRuleError(t *testing.T) {
	r1 := &stubRule{name: "fails", callErr: errors.New("deliberate failure")}
	r2 := &stubRule{name: "ok"}

	engine := NewDefaultRuleEngine()
	engine.Register(r1)
	engine.Register(r2)

	ctx := &AssemblyContext{
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            mappinglog.NewLogBuilder("doc"),
	}

	err := engine.Run(ctx)
	if err == nil {
		t.Error("expected error when a rule fails")
	}
	if !r2.called {
		t.Error("second rule should still run after first rule error")
	}
}

func TestDefaultRuleEngine_DisabledRule_Skipped(t *testing.T) {
	r1 := &stubRule{name: "skipped-rule"}

	engine := NewDefaultRuleEngine()
	engine.Register(r1)

	ctx := &AssemblyContext{
		DedupRedirects: make(map[string]string),
		Removed:        make(map[string]bool),
		Log:            mappinglog.NewLogBuilder("doc"),
		Config: AssemblyConfig{
			Rules: map[string]RuleConfig{
				"skipped-rule": {Disabled: true},
			},
		},
	}

	_ = engine.Run(ctx)
	if r1.called {
		t.Error("disabled rule should not be called")
	}
}

func TestDefaultRuleEngine_InitialisesNilMaps(t *testing.T) {
	engine := NewDefaultRuleEngine()
	engine.Register(&stubRule{name: "noop"})

	ctx := &AssemblyContext{Log: mappinglog.NewLogBuilder("doc")}
	// DedupRedirects and Removed are nil — engine should initialise them.
	if err := engine.Run(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if ctx.DedupRedirects == nil {
		t.Error("engine should initialise nil DedupRedirects")
	}
	if ctx.Removed == nil {
		t.Error("engine should initialise nil Removed")
	}
}
