package autocomplete

import "testing"

func TestApplyRootCompletion(t *testing.T) {
	var e Engine
	out := e.Apply("/pro", Options{})
	if out != "/provider " {
		t.Fatalf("expected /provider completion, got %q", out)
	}
}

func TestSuggestionsIncludeSessionIDs(t *testing.T) {
	var e Engine
	s := e.Suggestions("/session u", Options{SessionIDs: []string{"session-1"}})
	found := false
	for _, v := range s {
		if v == "/session use session-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected session suggestion")
	}
}
