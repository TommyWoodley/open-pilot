package autocomplete

import "testing"

func TestApplyRootCompletion(t *testing.T) {
	var e Engine
	out := e.Apply("/pro", Options{})
	if out != "/provider " {
		t.Fatalf("expected /provider completion, got %q", out)
	}
}

func TestSuggestionsIncludeSessionNames(t *testing.T) {
	var e Engine
	s := e.Suggestions("/session u", Options{SessionNames: []string{"test-session"}})
	found := false
	for _, v := range s {
		if v == "/session use test-session" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected session suggestion")
	}
}

func TestSuggestionsIncludeSessionDeleteByName(t *testing.T) {
	var e Engine
	s := e.Suggestions("/session d", Options{SessionNames: []string{"test-session"}})
	found := false
	for _, v := range s {
		if v == "/session delete test-session" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected session delete suggestion")
	}
}

func TestApplyHooksRunCompletion(t *testing.T) {
	var e Engine
	out := e.Apply("/hooks r", Options{})
	if out != "/hooks run " {
		t.Fatalf("expected /hooks run completion, got %q", out)
	}
}
