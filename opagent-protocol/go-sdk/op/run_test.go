package op

import "testing"

func TestRunValidateAllowsDaemonWithoutEndpoint(t *testing.T) {
	if err := (Run{Daemon: true}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestRunValidateRejectsCommandAndURLTogether(t *testing.T) {
	err := (Run{
		Command: []string{"./bin/coder"},
		URL:     "https://example.com/mcp",
	}).Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestRunValidateRejectsHeaderWithoutURL(t *testing.T) {
	err := (Run{
		Command: []string{"./bin/coder"},
		Header:  map[string]string{"Authorization": "Bearer token"},
	}).Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}
