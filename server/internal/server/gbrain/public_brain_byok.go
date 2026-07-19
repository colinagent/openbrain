package gbrain

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	hostcfg "github.com/colinagent/openbrain/server/internal/server/hostcfg"
	"github.com/gin-gonic/gin"
)

type cloudEvidenceEnvelope struct {
	KeyID     string `json:"keyId"`
	Algorithm string `json:"algorithm"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

type cloudEvidenceCitation struct {
	CitationID string `json:"citationId"`
	Title      string `json:"title"`
	Excerpt    string `json:"excerpt"`
}

type cloudEvidencePayload struct {
	Version        string                  `json:"version"`
	TurnID         string                  `json:"turnId"`
	ConversationID string                  `json:"conversationId"`
	BrainID        string                  `json:"brainId"`
	QuestionSHA256 string                  `json:"questionSHA256"`
	Citations      []cloudEvidenceCitation `json:"citations"`
	Funding        map[string]any          `json:"funding"`
	PricingVersion string                  `json:"pricingVersion"`
	IssuedAt       string                  `json:"issuedAt"`
	ExpiresAt      string                  `json:"expiresAt"`
}

type publicBrainBYOKTurnRequest struct {
	TurnID                     string                             `json:"turnId"`
	QuoteID                    string                             `json:"quoteId"`
	Question                   string                             `json:"question"`
	MaxAuthorizedDebitMicrousd int64                              `json:"maxAuthorizedDebitMicrousd"`
	ModelKey                   string                             `json:"modelKey"`
	History                    []op.RuntimeEvidenceHistoryMessage `json:"history,omitempty"`
}

type publicBrainBYOKCloudEvent struct {
	Type     string                 `json:"type"`
	Code     string                 `json:"code,omitempty"`
	Evidence *cloudEvidenceEnvelope `json:"evidence,omitempty"`
}

type runtimeEvidenceModelView struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

func (h *Handler) RuntimeModels(c *gin.Context) {
	host := hostcfg.Get()
	if host == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "runtime_unreachable"})
		return
	}
	cfg, err := host.GetConfigContext(c.Request.Context())
	if err != nil || cfg == nil || cfg.User == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "runtime_unreachable"})
		return
	}
	models, defaultModelKey := runtimeEvidenceModels(cfg.User)
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"models": models, "defaultModelKey": defaultModelKey})
}

func runtimeEvidenceModels(user *op.UserConfig) ([]runtimeEvidenceModelView, string) {
	if user == nil {
		return []runtimeEvidenceModelView{}, ""
	}
	models := make([]runtimeEvidenceModelView, 0, len(user.Models))
	available := map[string]struct{}{}
	for _, model := range user.Models {
		if !runtimeEvidenceModelAvailable(model) {
			continue
		}
		key := strings.TrimSpace(model.Key)
		models = append(models, runtimeEvidenceModelView{Key: key, Name: strings.TrimSpace(model.Name), Provider: strings.TrimSpace(model.Provider)})
		available[key] = struct{}{}
	}
	defaultModelKey := ""
	if user.Strategies != nil && user.Strategies.Auto != nil {
		candidate := strings.TrimSpace(user.Strategies.Auto.DefaultChatModelID)
		if _, ok := available[candidate]; ok {
			defaultModelKey = candidate
		}
	}
	return models, defaultModelKey
}

func runtimeEvidenceModelAvailable(model op.ModelConfig) bool {
	return strings.TrimSpace(model.Key) != "" && model.Enabled && strings.TrimSpace(model.APIKey) != "" && strings.TrimSpace(model.BaseURL) != "" &&
		!strings.EqualFold(strings.TrimSpace(model.Provider), "cloud") && !strings.EqualFold(strings.TrimSpace(model.Source), "gateway")
}

func (h *Handler) CloudRunPublicBrainBYOKTurn(c *gin.Context) {
	var input publicBrainBYOKTurnRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request"})
		return
	}
	input.TurnID = strings.TrimSpace(input.TurnID)
	input.QuoteID = strings.TrimSpace(input.QuoteID)
	input.Question = strings.TrimSpace(input.Question)
	input.ModelKey = strings.TrimSpace(input.ModelKey)
	if input.TurnID == "" || input.QuoteID == "" || input.Question == "" || input.ModelKey == "" || input.MaxAuthorizedDebitMicrousd < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request"})
		return
	}
	if err := validateBYOKRuntimeOnlyInput(input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "error": err.Error()})
		return
	}
	if err := preflightRuntimeEvidenceModel(c.Request.Context(), input.ModelKey); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "runtime_model_unavailable", "error": err.Error()})
		return
	}
	cloudBody, _ := marshalPublicBrainBYOKCloudTurn(input)
	endpoint := "/v1/public-brains/" + url.PathEscape(c.Param("brainID")) + "/conversations/" + url.PathEscape(c.Param("conversationID")) + "/turns"
	response, err := h.service.OpenCloudAPIStream(c.Request.Context(), http.MethodPost, endpoint, cloudBody)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "cloud_unavailable"})
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		c.Data(response.StatusCode, "application/json; charset=utf-8", raw)
		return
	}
	envelope, cloudCode, err := readCloudEvidenceEnvelope(response.Body)
	if err != nil {
		code := cloudCode
		if code == "" {
			code = "invalid_evidence"
		}
		c.JSON(http.StatusBadGateway, gin.H{"code": code})
		return
	}
	payload, err := h.verifyCloudEvidence(c.Request.Context(), envelope, c.Param("brainID"), c.Param("conversationID"), input.TurnID, input.Question)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": "invalid_evidence"})
		return
	}
	if len(payload.Citations) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"answer":    "This brain did not return enough verified evidence to answer.",
			"citations": payload.Citations, "funding": payload.Funding,
			"executionMode": "runtime_byok", "modelRan": false,
			"billingResponsibility": "external_provider", "evidenceCompleted": true,
		})
		return
	}
	evidence := make([]op.RuntimeEvidenceItem, 0, len(payload.Citations))
	for _, citation := range payload.Citations {
		evidence = append(evidence, op.RuntimeEvidenceItem{CitationID: citation.CitationID, Title: citation.Title, Excerpt: citation.Excerpt})
	}
	result, err := answerVerifiedEvidence(c.Request.Context(), op.RuntimeEvidenceAnswerRequest{
		RequestID: input.TurnID, ModelKey: input.ModelKey, Question: input.Question,
		Evidence: evidence, History: input.History,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"code": "runtime_provider_failed", "evidenceCompleted": true,
			"citations": payload.Citations, "funding": payload.Funding,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"answer": result.Answer, "citations": payload.Citations, "funding": payload.Funding,
		"executionMode": "runtime_byok", "modelKey": result.ModelKey, "modelRan": true,
		"billingResponsibility": result.BillingResponsibility, "evidenceCompleted": true,
	})
}

func validateBYOKRuntimeOnlyInput(input publicBrainBYOKTurnRequest) error {
	if len([]rune(input.Question)) > 1000 || len(input.ModelKey) > 256 || len(input.History) > 6 {
		return errors.New("runtime BYOK input exceeds its limit")
	}
	historyBytes := 0
	for _, message := range input.History {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		text := strings.TrimSpace(message.Text)
		if (role != "user" && role != "assistant") || text == "" {
			return errors.New("runtime BYOK history is invalid")
		}
		historyBytes += len(text)
	}
	if historyBytes > 8000 {
		return errors.New("runtime BYOK history is too large")
	}
	return nil
}

func marshalPublicBrainBYOKCloudTurn(input publicBrainBYOKTurnRequest) ([]byte, error) {
	return json.Marshal(map[string]any{
		"turnId": input.TurnID, "quoteId": input.QuoteID, "question": input.Question,
		"maxAuthorizedDebitMicrousd": input.MaxAuthorizedDebitMicrousd,
	})
}

func preflightRuntimeEvidenceModel(ctx context.Context, modelKey string) error {
	host := hostcfg.Get()
	if host == nil {
		return errors.New("active runtime is unreachable")
	}
	cfg, err := host.GetConfigContext(ctx)
	if err != nil || cfg == nil || cfg.User == nil {
		return errors.New("active runtime model configuration is unavailable")
	}
	for _, model := range cfg.User.Models {
		if strings.TrimSpace(model.Key) != modelKey {
			continue
		}
		if !runtimeEvidenceModelAvailable(model) {
			return errors.New("selected runtime model is not configured")
		}
		return nil
	}
	return errors.New("selected runtime model was not found")
}

func answerVerifiedEvidence(ctx context.Context, input op.RuntimeEvidenceAnswerRequest) (*op.RuntimeEvidenceAnswerResult, error) {
	host := hostcfg.Get()
	if host == nil || host.Session == nil {
		return nil, errors.New("active runtime is unreachable")
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	response, err := host.Session.OpNode(ctx, &op.OpNodeParams{
		OpCode: op.OpRuntimeEvidenceAnswer, Content: &op.JsonContent{Raw: raw},
	})
	if err != nil {
		return nil, err
	}
	content, ok := response.Content.(*op.JsonContent)
	if !ok || content == nil {
		return nil, errors.New("runtime evidence answer is invalid")
	}
	var result op.RuntimeEvidenceAnswerResult
	if err := content.Unmarshal(&result); err != nil {
		return nil, err
	}
	if strings.TrimSpace(result.Answer) == "" || result.BillingResponsibility != "external_provider" {
		return nil, errors.New("runtime evidence answer is invalid")
	}
	return &result, nil
}

func readCloudEvidenceEnvelope(reader io.Reader) (*cloudEvidenceEnvelope, string, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, 2<<20))
	scanner.Buffer(make([]byte, 16<<10), 1<<20)
	var data strings.Builder
	flush := func() (*cloudEvidenceEnvelope, string, error) {
		if data.Len() == 0 {
			return nil, "", nil
		}
		var event publicBrainBYOKCloudEvent
		err := json.Unmarshal([]byte(data.String()), &event)
		data.Reset()
		if err != nil {
			return nil, "", err
		}
		if event.Type == "error" {
			return nil, event.Code, errors.New("Cloud evidence turn failed")
		}
		if event.Type == "complete" && event.Evidence != nil {
			return event.Evidence, "", nil
		}
		return nil, "", nil
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if envelope, code, err := flush(); envelope != nil || err != nil {
				return envelope, code, err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, "", err
	}
	if envelope, code, err := flush(); envelope != nil || err != nil {
		return envelope, code, err
	}
	return nil, "", errors.New("Cloud turn returned no evidence")
}

func (h *Handler) verifyCloudEvidence(ctx context.Context, envelope *cloudEvidenceEnvelope, brainID, conversationID, turnID, question string) (*cloudEvidencePayload, error) {
	if envelope == nil || envelope.Algorithm != "Ed25519" || strings.TrimSpace(envelope.KeyID) == "" {
		return nil, errors.New("evidence envelope is invalid")
	}
	status, raw := h.service.ProxyCloudAPI(ctx, http.MethodGet, "/v1/public-brains/evidence-keys/"+url.PathEscape(envelope.KeyID), nil)
	if status != http.StatusOK {
		return nil, errors.New("evidence key is unavailable")
	}
	var key struct {
		KeyID     string `json:"keyId"`
		Algorithm string `json:"algorithm"`
		PublicKey string `json:"publicKey"`
	}
	if json.Unmarshal(raw, &key) != nil || key.KeyID != envelope.KeyID || key.Algorithm != "Ed25519" {
		return nil, errors.New("evidence key is invalid")
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(key.PublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return nil, errors.New("evidence key encoding is invalid")
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(envelope.Payload)
	if err != nil || len(payloadRaw) > 64<<10 {
		return nil, errors.New("evidence payload encoding is invalid")
	}
	signature, err := base64.RawURLEncoding.DecodeString(envelope.Signature)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(publicKey), payloadRaw, signature) {
		return nil, errors.New("evidence signature is invalid")
	}
	var payload cloudEvidencePayload
	decoder := json.NewDecoder(strings.NewReader(string(payloadRaw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return nil, errors.New("evidence payload is invalid")
	}
	questionHash := sha256.Sum256([]byte(strings.TrimSpace(question)))
	if payload.Version != "v1" || payload.TurnID != strings.TrimSpace(turnID) || payload.ConversationID != strings.TrimSpace(conversationID) ||
		payload.BrainID != strings.TrimSpace(brainID) || payload.QuestionSHA256 != hex.EncodeToString(questionHash[:]) || strings.TrimSpace(payload.PricingVersion) == "" {
		return nil, errors.New("evidence identity is invalid")
	}
	issuedAt, issueErr := time.Parse(time.RFC3339Nano, payload.IssuedAt)
	expiresAt, expiryErr := time.Parse(time.RFC3339Nano, payload.ExpiresAt)
	now := time.Now().UTC()
	if issueErr != nil || expiryErr != nil || issuedAt.After(now.Add(30*time.Second)) || !expiresAt.After(now) || expiresAt.Sub(issuedAt) > 5*time.Minute {
		return nil, errors.New("evidence lifetime is invalid")
	}
	for _, citation := range payload.Citations {
		if strings.TrimSpace(citation.CitationID) == "" || strings.TrimSpace(citation.Title) == "" || len(citation.Excerpt) > 12_000 {
			return nil, errors.New("evidence citation is invalid")
		}
	}
	return &payload, nil
}

func (h *Handler) CloudGetPublicBrainEvidenceKey(c *gin.Context) {
	h.proxyCloudAPI(c, http.MethodGet, "/v1/public-brains/evidence-keys/"+url.PathEscape(c.Param("keyID")))
}
