package op

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/xid"
)

type GeneralContent struct {
	Content Content `json:"content"`
	Meta    Meta    `json:"meta,omitempty"`
}

func (p *GeneralContent) UnmarshalJSON(data []byte) error {
	type params GeneralContent // avoid recursion
	var wire struct {
		params
		Content *wireContent `json:"content"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	// 只有当 content 不为 nil 时才解析
	if wire.Content != nil {
		var err error
		wire.params.Content, err = contentFromWire(wire.Content, nil)
		if err != nil {
			return err
		}
	}
	*p = GeneralContent(wire.params)
	return nil
}

func GenerateThreadID() string {
	now := time.Now().UTC()
	var suffix [3]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return fmt.Sprintf("thread-%s-%06x", now.Format("20060102T150405Z"), now.UnixNano()&0xffffff)
	}
	return fmt.Sprintf("thread-%s-%s", now.Format("20060102T150405Z"), hex.EncodeToString(suffix[:]))
}

func GenerateFileID() string {
	return fmt.Sprintf("file-%s", xid.New().String())
}

func GenerateTurnID() string {
	return fmt.Sprintf("turn-%s", xid.New().String())
}

func GenerateMessageID() string {
	return fmt.Sprintf("msg-%s", xid.New().String())
}

type LoopResult struct {
	LoopID           string `json:"loopID"`
	LoopName         string `json:"loopName"`
	ThreadID         string `json:"threadID"`
	AgentName        string `json:"agentName"`
	UserInput        string `json:"userInput"`
	TotalSteps       int64  `json:"totalSteps"`
	TotalTokens      int64  `json:"totalTokens"`
	CompletionTokens int64  `json:"completionTokens"`
	PromptTokens     int64  `json:"promptTokens"`
	Status           Status `json:"status"`
	Error            string `json:"error,omitempty"`
	Result           string `json:"result"`
}
