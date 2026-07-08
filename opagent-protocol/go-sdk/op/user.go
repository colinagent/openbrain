package op

// UserSettings stores per-user runtime preferences.
type UserSettings struct {
	UID            string `json:"uid" bson:"uid"`
	BaseDir        string `json:"baseDir,omitempty" bson:"baseDir,omitempty"`
	DefaultAgentID string `json:"defaultAgentID,omitempty" bson:"defaultAgentID,omitempty"`
	CreatedAt      int64  `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	UpdatedAt      int64  `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
}
