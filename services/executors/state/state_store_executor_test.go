// services/executors/state/state_store_executor_test.go
// Tests for state.store over an in-memory Store. Synthetic values only.
//
//   TC-ST-001  put then get with a matching token returns the value
//   TC-ST-002  a wrong token, an unknown key or an expired entry is missing
//   TC-ST-003  get without its "when" value does nothing (not missing)
//   TC-ST-004  put without the required value deletes the entry
//   TC-ST-005  delete; entries are scoped by pipeline and namespace
//   TC-ST-006  url_query keeps only a URL's query string
//   TC-ST-007  config validation
package state

import (
	"context"
	"sync"
	"testing"
	"time"

	"ezhealthkonnect/models"
)

type memEntry struct {
	value   map[string]interface{}
	expires time.Time
}

type memStore struct {
	mu   sync.Mutex
	m    map[string]memEntry
	now  func() time.Time
	puts int
}

func newMemStore() *memStore {
	return &memStore{m: map[string]memEntry{}, now: time.Now}
}

func (s *memStore) Put(_ context.Context, scope, key string, value map[string]interface{}, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.puts++
	s.m[scope+"\x00"+key] = memEntry{value, s.now().Add(ttl)}
	return nil
}

func (s *memStore) Get(_ context.Context, scope, key string) (map[string]interface{}, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[scope+"\x00"+key]
	if !ok || !s.now().Before(e.expires) {
		return nil, false, nil
	}
	return e.value, true, nil
}

func (s *memStore) Delete(_ context.Context, scope, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[scope+"\x00"+key]
	delete(s.m, scope+"\x00"+key)
	return ok, nil
}

func run(t *testing.T, ex *StateStoreExecutor, pipeline string, cfg map[string]interface{}, data map[string]interface{}) map[string]interface{} {
	t.Helper()
	step := &models.TransformationStep{PipelineID: pipeline, StepName: "State", StepType: "state.store", Enabled: true, Config: cfg}
	out, err := ex.Execute(context.Background(), step, data)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return out["_stepOutput"].(map[string]interface{})
}

var keyFrom = []interface{}{"q.app", "q.tag"}

func putCfg() map[string]interface{} {
	return map[string]interface{}{
		"operation": "put", "namespace": "pdq", "keyFrom": keyFrom, "token": true, "ttlSeconds": 60,
		"requireValue": "search",
		"values": []interface{}{
			map[string]interface{}{"name": "search", "path": "api.next", "transform": "url_query"},
			map[string]interface{}{"name": "tag", "path": "q.tag"},
		},
	}
}

func getCfg() map[string]interface{} {
	return map[string]interface{}{"operation": "get", "namespace": "pdq", "keyFrom": keyFrom, "when": "q.pointer", "matchToken": "q.pointer"}
}

func TestStateStore_PutGet(t *testing.T) {
	ex := NewStateStoreExecutor(newMemStore())
	q := map[string]interface{}{"app": "PDQCONSUMER", "tag": "Q-1"}
	put := run(t, ex, "p1", putCfg(), map[string]interface{}{"q": q,
		"api": map[string]interface{}{"next": "https://empi.example.org/fhir/r4/Patient?family=X&_count=5&_cursor=5"}})
	token, _ := put["token"].(string)
	if put["stored"] != true || len(token) != 32 {
		t.Fatalf("put %v", put)
	}
	q["pointer"] = token
	got := run(t, ex, "p1", getCfg(), map[string]interface{}{"q": q})
	v, _ := got["value"].(map[string]interface{})
	if got["found"] != true || got["missing"] != false || v["search"] != "family=X&_count=5&_cursor=5" || v["tag"] != "Q-1" {
		t.Fatalf("get %v", got)
	}
}

func TestStateStore_Missing(t *testing.T) {
	mem := newMemStore()
	ex := NewStateStoreExecutor(mem)
	q := map[string]interface{}{"app": "PDQCONSUMER", "tag": "Q-1"}
	put := run(t, ex, "p1", putCfg(), map[string]interface{}{"q": q, "api": map[string]interface{}{"next": "https://x.invalid/P?a=1"}})

	for name, pointer := range map[string]string{"wrong token": "0000", "right token": put["token"].(string)} {
		q["pointer"] = pointer
		got := run(t, ex, "p1", getCfg(), map[string]interface{}{"q": q})
		if wantMissing := name == "wrong token"; got["missing"] != wantMissing || got["found"] == wantMissing {
			t.Fatalf("%s: %v", name, got)
		}
	}
	// Unknown tag.
	got := run(t, ex, "p1", getCfg(), map[string]interface{}{"q": map[string]interface{}{"app": "PDQCONSUMER", "tag": "Q-2", "pointer": "x"}})
	if got["missing"] != true {
		t.Fatalf("unknown tag %v", got)
	}
	// Expired.
	mem.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	q["pointer"] = put["token"]
	if got := run(t, ex, "p1", getCfg(), map[string]interface{}{"q": q}); got["missing"] != true {
		t.Fatalf("expired %v", got)
	}
}

func TestStateStore_GetWithoutWhen(t *testing.T) {
	ex := NewStateStoreExecutor(newMemStore())
	got := run(t, ex, "p1", getCfg(), map[string]interface{}{"q": map[string]interface{}{"app": "A", "tag": "T"}})
	if got["found"] != false || got["missing"] != false {
		t.Fatalf("no pointer asked for: %v", got)
	}
}

func TestStateStore_PutWithoutRequiredValueDeletes(t *testing.T) {
	mem := newMemStore()
	ex := NewStateStoreExecutor(mem)
	q := map[string]interface{}{"app": "A", "tag": "T"}
	run(t, ex, "p1", putCfg(), map[string]interface{}{"q": q, "api": map[string]interface{}{"next": "https://x.invalid/P?a=1"}})
	if len(mem.m) != 1 {
		t.Fatal("not stored")
	}
	put := run(t, ex, "p1", putCfg(), map[string]interface{}{"q": q, "api": map[string]interface{}{}})
	if put["stored"] != false || put["token"] != nil || len(mem.m) != 0 {
		t.Fatalf("last page: %v, %d left", put, len(mem.m))
	}
}

func TestStateStore_DeleteAndScope(t *testing.T) {
	mem := newMemStore()
	ex := NewStateStoreExecutor(mem)
	q := map[string]interface{}{"app": "A", "tag": "T"}
	data := map[string]interface{}{"q": q, "api": map[string]interface{}{"next": "https://x.invalid/P?a=1"}}
	put := run(t, ex, "p1", putCfg(), data)
	q["pointer"] = put["token"]
	// Another pipeline doesn't see it.
	if got := run(t, ex, "p2", getCfg(), map[string]interface{}{"q": q}); got["missing"] != true {
		t.Fatalf("other pipeline %v", got)
	}
	del := map[string]interface{}{"operation": "delete", "namespace": "pdq", "keyFrom": keyFrom}
	if got := run(t, ex, "p1", del, map[string]interface{}{"q": q}); got["deleted"] != true {
		t.Fatalf("delete %v", got)
	}
	if got := run(t, ex, "p1", del, map[string]interface{}{"q": q}); got["deleted"] != false {
		t.Fatalf("second delete %v", got)
	}
}

func TestStateStore_URLQuery(t *testing.T) {
	for in, want := range map[string]string{
		"https://empi.example.org/fhir/r4/Patient?family=X&_cursor=5": "family=X&_cursor=5",
		"https://empi.example.org/fhir/r4/Patient":                    "",
		"":                                                            "",
	} {
		if got := valueTransforms["url_query"](in); got != want {
			t.Errorf("url_query(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStateStore_Validate(t *testing.T) {
	ex := NewStateStoreExecutor(newMemStore())
	for _, cfg := range []map[string]interface{}{
		{"operation": "drop", "keyFrom": keyFrom},
		{"operation": "get"},
	} {
		if err := ex.Validate(&models.TransformationStep{Config: cfg}); err == nil {
			t.Errorf("accepted %v", cfg)
		}
	}
	if err := ex.Validate(&models.TransformationStep{Config: getCfg()}); err != nil {
		t.Errorf("refused a valid config: %v", err)
	}
	// No store: a clear error, not a panic.
	step := &models.TransformationStep{StepName: "State", StepType: "state.store", Enabled: true, Config: getCfg()}
	if _, err := NewStateStoreExecutor(nil).Execute(context.Background(), step, map[string]interface{}{}); err == nil {
		t.Error("ran without a store")
	}
}
