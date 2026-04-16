package config

import (
	"encoding/json"
	"errors"
	"testing"
)

// newTestManager creates a Manager with pre-populated values,
// bypassing the DB-dependent NewManager constructor.
func newTestManager(values map[string]json.RawMessage) *Manager {
	m := &Manager{
		values: values,
	}
	return m
}

func TestGet_Missing(t *testing.T) {
	m := newTestManager(map[string]json.RawMessage{})

	var result string
	err := m.Get("nonexistent", &result)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing key: got err=%v, want ErrNotFound", err)
	}
}

func TestGet_Found(t *testing.T) {
	m := newTestManager(map[string]json.RawMessage{
		"camera.gain": json.RawMessage(`300`),
	})

	var gain int
	err := m.Get("camera.gain", &gain)
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if gain != 300 {
		t.Errorf("Get camera.gain: got %d, want 300", gain)
	}
}

func TestGet_StringValue(t *testing.T) {
	m := newTestManager(map[string]json.RawMessage{
		"camera.name": json.RawMessage(`"ZWO ASI676MC"`),
	})

	var name string
	err := m.Get("camera.name", &name)
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if name != "ZWO ASI676MC" {
		t.Errorf("Get camera.name: got %q, want \"ZWO ASI676MC\"", name)
	}
}

func TestGet_StructValue(t *testing.T) {
	type Profile struct {
		Gain      int     `json:"gain"`
		ADUTarget float64 `json:"aduTarget"`
	}

	m := newTestManager(map[string]json.RawMessage{
		"profile.night": json.RawMessage(`{"gain":300,"aduTarget":25.0}`),
	})

	var p Profile
	err := m.Get("profile.night", &p)
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if p.Gain != 300 || p.ADUTarget != 25.0 {
		t.Errorf("Get profile.night: got %+v, want {Gain:300 ADUTarget:25}", p)
	}
}

func TestGetRaw(t *testing.T) {
	m := newTestManager(map[string]json.RawMessage{
		"key": json.RawMessage(`{"a":1}`),
	})

	raw := m.GetRaw("key")
	if string(raw) != `{"a":1}` {
		t.Errorf("GetRaw: got %s, want {\"a\":1}", string(raw))
	}

	// Missing key returns nil
	raw = m.GetRaw("missing")
	if raw != nil {
		t.Errorf("GetRaw missing: got %s, want nil", string(raw))
	}
}

func TestGetAll(t *testing.T) {
	m := newTestManager(map[string]json.RawMessage{
		"a": json.RawMessage(`1`),
		"b": json.RawMessage(`2`),
	})

	all := m.GetAll()
	if len(all) != 2 {
		t.Errorf("GetAll: got %d entries, want 2", len(all))
	}

	// Verify it returns a copy (modifying returned map does not affect manager)
	delete(all, "a")
	if m.GetRaw("a") == nil {
		t.Error("GetAll should return a copy, not the internal map")
	}
}
