package provider

import "github.com/colinagent/openbrain/opagent-runtime/packages/ai"

func ensureCanonicalContentIndex(msg *ai.StreamConversationMessage, index int, blockType ai.ContentBlockType) {
	if msg == nil || index < 0 {
		return
	}
	for len(msg.Content) <= index {
		msg.Content = append(msg.Content, ai.StreamContentBlock{})
	}
	msg.Content[index].Type = blockType
}
