package op

type RuntimeEvidenceItem struct {
	CitationID string `json:"citationId"`
	Title      string `json:"title"`
	Excerpt    string `json:"excerpt"`
}

type RuntimeEvidenceHistoryMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// RuntimeEvidenceAnswerRequest contains only already-verified, scrubbed
// evidence. ModelKey is resolved by the active runtime; provider credentials
// are never part of this request or any Cloud API request.
type RuntimeEvidenceAnswerRequest struct {
	RequestID string                          `json:"requestId"`
	ModelKey  string                          `json:"modelKey"`
	Question  string                          `json:"question"`
	Evidence  []RuntimeEvidenceItem           `json:"evidence"`
	History   []RuntimeEvidenceHistoryMessage `json:"history,omitempty"`
}

type RuntimeEvidenceAnswerResult struct {
	RequestID             string `json:"requestId"`
	Answer                string `json:"answer"`
	ModelKey              string `json:"modelKey"`
	BillingResponsibility string `json:"billingResponsibility"`
}
