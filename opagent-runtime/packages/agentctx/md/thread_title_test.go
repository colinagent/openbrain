package md

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSlugifyThreadTitleUnicodeAndSafety(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "keeps_chinese", input: "非常", want: "非常"},
		{name: "unicode_whitespace_to_dash", input: "非常  你好", want: "非常-你好"},
		{name: "slash_replaced", input: "a/b", want: "a-b"},
		{name: "windows_reserved_name", input: "CON", want: "con-"},
		{name: "unsafe_only_fallback", input: "<>:\"/\\|?*", want: "untitled-chat"},
		{name: "dot_names_fallback", input: "..", want: "untitled-chat"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SlugifyThreadTitle(tc.input)
			if got != tc.want {
				t.Fatalf("SlugifyThreadTitle(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSlugifyThreadTitleRuneSafeTruncation(t *testing.T) {
	t.Parallel()

	input := strings.Repeat("你", maxThreadSlugLength+8)
	got := SlugifyThreadTitle(input)
	if !utf8.ValidString(got) {
		t.Fatalf("slug is not valid UTF-8: %q", got)
	}
	if got == "" {
		t.Fatalf("slug should not be empty")
	}
	if utf8.RuneCountInString(got) != maxThreadSlugLength {
		t.Fatalf("slug rune length = %d, want %d", utf8.RuneCountInString(got), maxThreadSlugLength)
	}
}

func TestDeriveChatTitleSkipsStandaloneReferencesAndImages(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		"[codex](/Users/example/code/OpAgent/third_party_refs/codex)",
		"![image-1](./assets/image-1.png){width=10%}",
		"",
		"我要做这个功能",
	}, "\n")

	if got := DeriveChatTitle(input); got != "我要做这个功能" {
		t.Fatalf("DeriveChatTitle() = %q, want %q", got, "我要做这个功能")
	}
}

func TestDeriveChatTitleNormalizesInlineMarkdownLinks(t *testing.T) {
	t.Parallel()

	input := `参考 [codex\]](/Users/example/code/OpAgent/third_party_refs/codex\)) 设计`

	if got := DeriveChatTitle(input); got != `参考 codex] 设计` {
		t.Fatalf("DeriveChatTitle() = %q, want %q", got, `参考 codex] 设计`)
	}
}

func TestDeriveChatTitleFallsBackForReferenceOnlyInput(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		"[codex](/Users/example/code/OpAgent/third_party_refs/codex)",
		"",
		"![image-1](./assets/image-1.png){width=10%}",
	}, "\n")

	if got := DeriveChatTitle(input); got != defaultThreadTitle {
		t.Fatalf("DeriveChatTitle() = %q, want %q", got, defaultThreadTitle)
	}
}

// func TestBuildUniqueChatPathKeepsUnicodeFilename(t *testing.T) {
// 	t.Parallel()

// 	cwd := t.TempDir()
// 	got, err := buildUniqueChatPath(cwd, "非常")
// 	if err != nil {
// 		t.Fatalf("buildUniqueChatPath() error = %v", err)
// 	}
// 	want := filepath.Join(cwd, ".agent", "chat", "非常.md")
// 	if got != want {
// 		t.Fatalf("buildUniqueChatPath() = %q, want %q", got, want)
// 	}
// }
