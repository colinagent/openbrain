package gbrain

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestReadCloudEvidenceEnvelopeFindsOnlyCompleteEvidence(t *testing.T) {
	stream := "event: accepted\ndata: {\"type\":\"accepted\"}\n\n" +
		"event: complete\ndata: {\"type\":\"complete\",\"evidence\":{\"keyId\":\"evidence-test\",\"algorithm\":\"Ed25519\",\"payload\":\"payload\",\"signature\":\"signature\"}}\n\n"
	envelope, code, err := readCloudEvidenceEnvelope(strings.NewReader(stream))
	if err != nil || code != "" || envelope == nil || envelope.KeyID != "evidence-test" {
		t.Fatalf("envelope/code/error = %+v/%q/%v", envelope, code, err)
	}
}

func TestBYOKCloudTurnNeverContainsModelKeyOrLocalHistory(t *testing.T) {
	raw, err := marshalPublicBrainBYOKCloudTurn(publicBrainBYOKTurnRequest{
		TurnID: "turn-test", QuoteID: "quote-test", Question: "question", MaxAuthorizedDebitMicrousd: 455,
		ModelKey: "private-provider:model", History: []op.RuntimeEvidenceHistoryMessage{{Role: "assistant", Text: "local answer"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private-provider") || strings.Contains(string(raw), "local answer") || strings.Contains(string(raw), "modelKey") || strings.Contains(string(raw), "history") {
		t.Fatalf("Cloud BYOK request leaked runtime-only data: %s", raw)
	}
}

func TestBYOKRuntimeOnlyInputIsRejectedBeforeCloudRetrieval(t *testing.T) {
	input := publicBrainBYOKTurnRequest{Question: "question", ModelKey: "user:model"}
	for index := 0; index < 7; index++ {
		input.History = append(input.History, op.RuntimeEvidenceHistoryMessage{Role: "user", Text: "history"})
	}
	if err := validateBYOKRuntimeOnlyInput(input); err == nil {
		t.Fatal("oversized local history was accepted")
	}
}

func TestRuntimeEvidenceModelListIsSafeAndRuntimeLocal(t *testing.T) {
	models, defaultKey := runtimeEvidenceModels(&op.UserConfig{
		Models: []op.ModelConfig{
			{Key: "user:model", Name: "User Model", Provider: "user", APIKey: "secret", BaseURL: "https://private.example", Enabled: true},
			{Key: "cloud:model", Name: "Managed", Provider: "cloud", Source: "gateway", APIKey: "managed-secret", BaseURL: "https://managed.example", Enabled: true},
		},
		Strategies: &op.ModelStrategies{Auto: &op.ModelAutoStrategy{DefaultChatModelID: "user:model"}},
	})
	if len(models) != 1 || models[0].Key != "user:model" || defaultKey != "user:model" {
		t.Fatalf("runtime evidence models/default = %+v/%q", models, defaultKey)
	}
	raw, _ := json.Marshal(models)
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "private.example") || strings.Contains(string(raw), "apiKey") || strings.Contains(string(raw), "baseURL") {
		t.Fatalf("safe runtime model view leaked credentials: %s", raw)
	}
}

func TestCloudEvidenceSignatureBindsQuestionAndIdentity(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	questionHash := sha256.Sum256([]byte("question"))
	payload := cloudEvidencePayload{
		Version: "v1", TurnID: "turn-test", ConversationID: "conversation-test", BrainID: "brain-test",
		QuestionSHA256: hex.EncodeToString(questionHash[:]), PricingVersion: "pricing-v1",
		Citations: []cloudEvidenceCitation{{CitationID: "pbcit-test", Title: "Evidence", Excerpt: "Safe"}},
		Funding:   map[string]any{"kind": "ai_balance"}, IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(5 * time.Minute).Format(time.RFC3339Nano),
	}
	raw, _ := json.Marshal(payload)
	envelope := cloudEvidenceEnvelope{
		KeyID: "evidence-test", Algorithm: "Ed25519", Payload: base64.RawURLEncoding.EncodeToString(raw),
		Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, raw)),
	}
	if !ed25519.Verify(publicKey, raw, mustDecodeBase64URL(t, envelope.Signature)) {
		t.Fatal("valid evidence signature did not verify")
	}
	tampered := append([]byte(nil), raw...)
	tampered[len(tampered)-1] ^= 1
	if ed25519.Verify(publicKey, tampered, mustDecodeBase64URL(t, envelope.Signature)) {
		t.Fatal("tampered evidence signature verified")
	}
}

func mustDecodeBase64URL(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
