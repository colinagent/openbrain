package common

import "testing"

func TestUnmarshalJSONCAllowsLineAndBlockComments(t *testing.T) {
	raw := []byte(`{
  // line comment
  "url": "https://example.com/docs // keep this text",
  /* block comment */
  "text": "not a /* comment */"
}`)
	var out struct {
		URL  string `json:"url"`
		Text string `json:"text"`
	}
	if err := UnmarshalJSONC(raw, &out); err != nil {
		t.Fatalf("UnmarshalJSONC(): %v", err)
	}
	if out.URL != "https://example.com/docs // keep this text" {
		t.Fatalf("URL = %q", out.URL)
	}
	if out.Text != "not a /* comment */" {
		t.Fatalf("Text = %q", out.Text)
	}
}

func TestUnmarshalJSONCRejectsUnterminatedBlockComment(t *testing.T) {
	var out map[string]any
	if err := UnmarshalJSONC([]byte(`{"ok": true /*`), &out); err == nil {
		t.Fatal("UnmarshalJSONC() error = nil, want error")
	}
}

func TestUnmarshalJSONCDoesNotAllowTrailingCommas(t *testing.T) {
	var out map[string]any
	if err := UnmarshalJSONC([]byte(`{"ok": true,}`), &out); err == nil {
		t.Fatal("UnmarshalJSONC() error = nil, want error")
	}
}
