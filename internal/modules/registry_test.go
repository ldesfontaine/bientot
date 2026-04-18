package modules

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeModule implements Module for testing.
type fakeModule struct{ name string }

func (f *fakeModule) Name() string                           { return f.name }
func (f *fakeModule) Detect(context.Context) error           { return nil }
func (f *fakeModule) Collect(context.Context) (*Data, error) { return nil, nil }
func (f *fakeModule) Interval() time.Duration                { return time.Minute }

func fakeFactory(name string) FactoryFunc {
	return func(_ map[string]interface{}) (Module, error) {
		return &fakeModule{name: name}, nil
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}

func TestBuild_TwoModules(t *testing.T) {
	registry = make(map[string]FactoryFunc)
	Register("fake-a", fakeFactory("a"))
	Register("fake-b", fakeFactory("b"))

	configs := []ModuleConfig{
		{Type: "fake-a", Enabled: true},
		{Type: "fake-b", Enabled: true},
	}

	result, err := Build(configs, testLogger())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d modules, want 2", len(result))
	}
}

func TestBuild_DisabledSkipped(t *testing.T) {
	registry = make(map[string]FactoryFunc)
	Register("fake", fakeFactory("fake"))

	configs := []ModuleConfig{
		{Type: "fake", Enabled: false},
	}

	result, err := Build(configs, testLogger())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("disabled module was included")
	}
}

func TestBuild_UnknownType(t *testing.T) {
	registry = make(map[string]FactoryFunc)
	Register("known", fakeFactory("known"))

	configs := []ModuleConfig{
		{Type: "unknown", Enabled: true},
	}

	_, err := Build(configs, testLogger())
	if err == nil {
		t.Fatal("expected error for unknown type, got nil")
	}
	if !strings.Contains(err.Error(), "known") {
		t.Errorf("error should list known types, got: %v", err)
	}
}

func TestBuild_FactoryError(t *testing.T) {
	registry = make(map[string]FactoryFunc)
	Register("bad", func(_ map[string]interface{}) (Module, error) {
		return nil, &buildErr{"construct failed"}
	})

	configs := []ModuleConfig{{Type: "bad", Enabled: true}}

	_, err := Build(configs, testLogger())
	if err == nil {
		t.Fatal("expected factory error, got nil")
	}
	if !strings.Contains(err.Error(), "construct failed") {
		t.Errorf("error should wrap factory error, got: %v", err)
	}
}

func TestRegister_DoubleRegisterPanics(t *testing.T) {
	registry = make(map[string]FactoryFunc)
	Register("dup", fakeFactory("dup"))

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on double Register, got none")
		}
	}()
	Register("dup", fakeFactory("dup"))
}

type buildErr struct{ msg string }

func (e *buildErr) Error() string { return e.msg }
