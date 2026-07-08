package core

import (
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

const (
	messagePublishToolName   = "message_publish"
	messageUpdateToolName    = "message_update"
	messageReadToolName      = "message_read"
	messageSubscribeToolName = "message_subscribe"
	messageAckToolName       = "message_ack"
)

func defaultMessageChannelID(threadID, agentID string) string {
	threadID = strings.TrimSpace(threadID)
	agentID = strings.TrimSpace(agentID)
	switch {
	case threadID != "" && agentID != "":
		return "channel-" + threadID + "-" + agentID
	case threadID != "":
		return "channel-" + threadID
	case agentID != "":
		return "channel-" + agentID
	default:
		return "channel-default"
	}
}

func normalizeMessageKind(kind op.MessageKind) op.MessageKind {
	switch kind {
	case op.MessageKindRequest, op.MessageKindStatus:
		return kind
	default:
		return op.MessageKindMessage
	}
}

func normalizeMessageStatus(status op.MessageStatus) op.MessageStatus {
	switch status {
	case op.MessageStatusResolved, op.MessageStatusArchived:
		return status
	default:
		return op.MessageStatusOpen
	}
}

func normalizeMessageSender(sender op.MessageSender) op.MessageSender {
	switch sender {
	case op.MessageSenderUser, op.MessageSenderSystem:
		return sender
	default:
		return op.MessageSenderAgent
	}
}

func normalizeMessageActionTone(tone op.MessageActionTone) op.MessageActionTone {
	switch tone {
	case op.MessageActionToneDanger:
		return op.MessageActionToneDanger
	case op.MessageActionTonePrimary:
		return op.MessageActionTonePrimary
	default:
		return ""
	}
}

func normalizeMessageActions(actions []op.MessageAction) []op.MessageAction {
	if len(actions) == 0 {
		return nil
	}
	out := make([]op.MessageAction, 0, len(actions))
	for _, action := range actions {
		id := strings.TrimSpace(action.ID)
		label := strings.TrimSpace(action.Label)
		if id == "" || label == "" {
			continue
		}
		out = append(out, op.MessageAction{
			ID:    id,
			Label: label,
			Tone:  normalizeMessageActionTone(action.Tone),
		})
	}
	return out
}

func normalizeMessageQuestions(questions []op.MessageQuestion) []op.MessageQuestion {
	if len(questions) == 0 {
		return nil
	}
	out := make([]op.MessageQuestion, 0, len(questions))
	for _, question := range questions {
		id := strings.TrimSpace(question.ID)
		body := strings.TrimSpace(question.Question)
		if id == "" || body == "" {
			continue
		}
		options := make([]op.MessageQuestionOption, 0, len(question.Options))
		for _, option := range question.Options {
			optionID := strings.TrimSpace(option.ID)
			label := strings.TrimSpace(option.Label)
			if optionID == "" || label == "" {
				continue
			}
			options = append(options, op.MessageQuestionOption{
				ID:    optionID,
				Label: label,
			})
		}
		out = append(out, op.MessageQuestion{
			ID:       id,
			Question: body,
			Options:  options,
		})
	}
	return out
}

func normalizeMessageAnswers(answers []op.MessageAnswer) []op.MessageAnswer {
	if len(answers) == 0 {
		return nil
	}
	out := make([]op.MessageAnswer, 0, len(answers))
	for _, answer := range answers {
		questionID := strings.TrimSpace(answer.QuestionID)
		optionID := strings.TrimSpace(answer.OptionID)
		label := strings.TrimSpace(answer.Label)
		text := strings.TrimSpace(answer.Text)
		if questionID == "" || (optionID == "" && label == "" && !answer.Other && text == "") {
			continue
		}
		out = append(out, op.MessageAnswer{
			QuestionID: questionID,
			OptionID:   optionID,
			Label:      label,
			Other:      answer.Other,
			Text:       text,
		})
	}
	return out
}

func cloneMessageRecord(src op.MessageRecord) op.MessageRecord {
	next := src
	next.Actions = normalizeMessageActions(src.Actions)
	next.Questions = normalizeMessageQuestions(src.Questions)
	next.Answers = normalizeMessageAnswers(src.Answers)
	if src.Meta != nil {
		next.Meta = src.Meta.Clone()
	}
	return next
}

func cloneMessageRecords(input []op.MessageRecord) []op.MessageRecord {
	if len(input) == 0 {
		return nil
	}
	out := make([]op.MessageRecord, 0, len(input))
	for _, record := range input {
		out = append(out, cloneMessageRecord(record))
	}
	return out
}
