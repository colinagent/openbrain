package op

import (
	"encoding/json"
	"fmt"
)

// ThreadEntry is a durable JSONL thread entry. It exposes common entry metadata
// for revision/dedup logic while preserving the original wire object.
type ThreadEntry struct {
	Type      string          `json:"type,omitempty"`
	ID        string          `json:"id,omitempty"`
	ParentID  *string         `json:"parentId,omitempty"`
	Timestamp string          `json:"timestamp,omitempty"`
	Raw       json.RawMessage `json:"-"`
}

func (entry *ThreadEntry) UnmarshalJSON(raw []byte) error {
	var base ThreadEntryBase
	if err := json.Unmarshal(raw, &base); err != nil {
		return err
	}
	entry.Type = base.Type
	entry.ID = base.ID
	entry.ParentID = base.ParentID
	entry.Timestamp = base.Timestamp
	entry.Raw = append(entry.Raw[:0], raw...)
	return nil
}

func (entry ThreadEntry) MarshalJSON() ([]byte, error) {
	if len(entry.Raw) > 0 {
		if !json.Valid(entry.Raw) {
			return nil, fmt.Errorf("invalid thread entry raw json")
		}
		return append([]byte(nil), entry.Raw...), nil
	}
	type threadEntry ThreadEntry
	return json.Marshal(threadEntry(entry))
}

func DecodeThreadEntry(raw []byte) (ThreadEntry, error) {
	var entry ThreadEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return ThreadEntry{}, err
	}
	return entry, nil
}
