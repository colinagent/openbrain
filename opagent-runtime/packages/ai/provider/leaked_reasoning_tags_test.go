package provider

import "testing"

func TestSanitizeTaggedAssistantContentExtractsReasoning(t *testing.T) {
	visible, reasoning, hadTags := sanitizeTaggedAssistantContent("Before<thinking>secret</thinking>After")
	if visible != "BeforeAfter" {
		t.Fatalf("visible = %q, want %q", visible, "BeforeAfter")
	}
	if reasoning != "secret" {
		t.Fatalf("reasoning = %q, want %q", reasoning, "secret")
	}
	if !hadTags {
		t.Fatalf("expected hadTags to be true")
	}
}

func TestSanitizeTaggedAssistantContentStripsFinalWrapper(t *testing.T) {
	visible, reasoning, hadTags := sanitizeTaggedAssistantContent("<thinking>secret</thinking><final>Visible</final>")
	if visible != "Visible" {
		t.Fatalf("visible = %q, want %q", visible, "Visible")
	}
	if reasoning != "secret" {
		t.Fatalf("reasoning = %q, want %q", reasoning, "secret")
	}
	if !hadTags {
		t.Fatalf("expected hadTags to be true")
	}
}

func TestSanitizeTaggedAssistantContentPreservesCodeExamples(t *testing.T) {
	input := "Example:\n```\n<thinking>code</thinking>\n```\nUse `<thinking>` literally."
	visible, reasoning, hadTags := sanitizeTaggedAssistantContent(input)
	if visible != input {
		t.Fatalf("visible = %q, want original input", visible)
	}
	if reasoning != "" {
		t.Fatalf("reasoning = %q, want empty", reasoning)
	}
	if hadTags {
		t.Fatalf("expected hadTags to be false for code-only examples")
	}
}

func TestLeakedReasoningStreamFilterHandlesChunkedTags(t *testing.T) {
	filter := newLeakedReasoningStreamFilter()

	visible, reasoning := filter.Consume("<thin")
	if visible != "" || reasoning != "" {
		t.Fatalf("first chunk visible=%q reasoning=%q, want both empty", visible, reasoning)
	}

	visible, reasoning = filter.Consume("king>secret</thinking>Visible")
	if visible != "Visible" {
		t.Fatalf("visible = %q, want %q", visible, "Visible")
	}
	if reasoning != "secret" {
		t.Fatalf("reasoning = %q, want %q", reasoning, "secret")
	}

	tailVisible, tailReasoning := filter.Finalize()
	if tailVisible != "" || tailReasoning != "" {
		t.Fatalf("finalize visible=%q reasoning=%q, want both empty", tailVisible, tailReasoning)
	}
}
