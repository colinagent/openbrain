package op

import (
	"regexp"
	"testing"
)

func TestGenerateThreadIDUsesDatePrefixAndHexSuffix(t *testing.T) {
	threadID := GenerateThreadID()
	matched, err := regexp.MatchString(`^thread-\d{8}T\d{6}Z-[0-9a-f]{6}$`, threadID)
	if err != nil {
		t.Fatalf("compile regexp: %v", err)
	}
	if !matched {
		t.Fatalf("GenerateThreadID() = %q, want thread-YYYYMMDDTHHMMSSZ-<6 hex>", threadID)
	}
}
