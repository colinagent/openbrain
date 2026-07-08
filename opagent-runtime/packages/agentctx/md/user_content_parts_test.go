package md

import (
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestFormatUserContentPartsUsesChatImageDefaultWidth(t *testing.T) {
	t.Parallel()

	got := formatUserContentParts([]op.ContentPart{
		{Type: "text", Text: "look"},
		{
			Type: "image_url",
			ImageURL: &op.ImageURL{
				URL: "file:///tmp/image.png",
			},
		},
	})

	if !strings.Contains(got, "![image](file:///tmp/image.png){width=10%}") {
		t.Fatalf("formatUserContentParts() = %q, want image width 10%%", got)
	}
}
