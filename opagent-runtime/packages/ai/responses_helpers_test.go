package ai

import "testing"

func TestResponseContentTexts(t *testing.T) {
	got := ResponseContentTexts([]ResponseContentPart{
		{Type: "output_text", Text: "  hello  "},
		{Type: "output_text", Text: ""},
		{Type: "output_text", Text: " world "},
	})
	if len(got) != 2 || got[0] != "hello" || got[1] != "world" {
		t.Fatalf("ResponseContentTexts() = %#v, want [hello world]", got)
	}
}
