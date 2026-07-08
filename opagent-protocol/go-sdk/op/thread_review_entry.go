package op

type ThreadReviewEntry struct {
	ThreadEntryBase
	TurnID string                 `json:"turnID"`
	Status ThreadReviewTurnStatus `json:"status"`
}
