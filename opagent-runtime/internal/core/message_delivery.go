package core

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func OpMessageListHandler(req *op.OpAgentRequest) (*op.OpAgentResult, error) {
	var params op.MessageListParams
	if err := decodeOpAgentJSONContent(req, &params); err != nil {
		return nil, err
	}
	result, err := defaultMessageStore.list(params)
	if err != nil {
		return nil, err
	}
	return messageOpAgentResult(req.Params.Meta, op.OpMessageList, result)
}

func OpMessageReadHandler(req *op.OpAgentRequest) (*op.OpAgentResult, error) {
	var params op.MessageReadParams
	if err := decodeOpAgentJSONContent(req, &params); err != nil {
		return nil, err
	}
	result, err := defaultMessageStore.read(params)
	if err != nil {
		return nil, err
	}
	return messageOpAgentResult(req.Params.Meta, op.OpMessageRead, result)
}

func OpMessageReplyHandler(req *op.OpAgentRequest) (*op.OpAgentResult, error) {
	var params op.MessageReplyParams
	if err := decodeOpAgentJSONContent(req, &params); err != nil {
		return nil, err
	}
	record, resolved, err := defaultMessageStore.replyWithResolved(params)
	if err != nil {
		return nil, err
	}
	if resolved != nil {
		_ = notifyMessageRecord(req.Params.Meta, *resolved)
	}
	_ = notifyMessageRecord(req.Params.Meta, record)
	result := op.MessageReplyResult{Record: record, Resolved: resolved}
	dispatch, queueAck, err := queueMessageReplyDispatch(req.Params.Meta, record, resolved)
	if err != nil {
		return nil, err
	}
	result.Dispatch = dispatch
	result.Queue = queueAck
	return messageOpAgentResult(req.Params.Meta, op.OpMessageReply, result)
}

func OpMessageAckHandler(req *op.OpAgentRequest) (*op.OpAgentResult, error) {
	var params op.MessageAckParams
	if err := decodeOpAgentJSONContent(req, &params); err != nil {
		return nil, err
	}
	result, err := defaultMessageStore.ack(params)
	if err != nil {
		return nil, err
	}
	return messageOpAgentResult(req.Params.Meta, op.OpMessageAck, result)
}

func OpMessageArchiveHandler(req *op.OpAgentRequest) (*op.OpAgentResult, error) {
	var params op.MessageArchiveParams
	if err := decodeOpAgentJSONContent(req, &params); err != nil {
		return nil, err
	}
	result, err := defaultMessageStore.archive(params)
	if err != nil {
		return nil, err
	}
	return messageOpAgentResult(req.Params.Meta, op.OpMessageArchive, result)
}

func decodeOpAgentJSONContent(req *op.OpAgentRequest, out any) error {
	if req == nil || req.Params == nil {
		return fmt.Errorf("agent request params are required")
	}
	if req.Params.Content == nil {
		return nil
	}
	jsonContent, ok := req.Params.Content.(*op.JsonContent)
	if !ok {
		return fmt.Errorf("content must be json")
	}
	if len(jsonContent.Raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(jsonContent.Raw, out); err != nil {
		return fmt.Errorf("decode %s content: %w", strings.TrimSpace(string(req.Params.OpCode)), err)
	}
	return nil
}

func messageOpAgentResult(meta op.Meta, opcode op.OpCode, payload any) (*op.OpAgentResult, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal %s result: %w", opcode, err)
	}
	return &op.OpAgentResult{
		OpCode:  opcode,
		Meta:    meta.Clone(),
		Content: &op.JsonContent{Raw: raw},
	}, nil
}

func queueMessageReplyDispatch(requestMeta op.Meta, record op.MessageRecord, resolved *op.MessageRecord) (*op.MessageReplyDispatch, *op.ThreadControlAck, error) {
	threadID := strings.TrimSpace(record.ThreadID)
	agentID := strings.TrimSpace(record.AgentID)
	if threadID == "" || agentID == "" {
		return nil, nil, nil
	}
	threadMeta, err := getThreadMeta(threadID, "")
	if err != nil {
		if isThreadNotFound(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	queueMeta := *threadMeta
	queueMeta.AgentID = agentID
	if cwd := strings.TrimSpace(metaString(requestMeta, "cwd")); cwd != "" {
		queueMeta.CWD = cwd
	}
	msg := messageReplyQueueMessage(record, resolved)
	_, snapshot, err := appendQueuedMessageToThread(
		queueMeta,
		op.ThreadQueueKindFollowUp,
		msg,
		"",
		metaString(requestMeta, "modelKey"),
		metaString(requestMeta, "thinkingLevel"),
		metaPositiveInt64(requestMeta, "contextWindow"),
		metaServiceTier(requestMeta),
		nil,
		messageReplySelectedSkillContext(record, resolved),
		false,
	)
	if err != nil {
		return nil, nil, err
	}
	meta := applyResolvedThreadMetaToMeta(requestMeta, *threadMeta)
	if meta == nil {
		meta = op.Meta{}
	}
	meta["opcode"] = string(op.OpThreadSubmit)
	meta["agentID"] = agentID
	meta["threadID"] = threadID
	if channelID := strings.TrimSpace(record.ChannelID); channelID != "" {
		meta["channelID"] = channelID
	}
	if messageID := strings.TrimSpace(record.ID); messageID != "" {
		meta["messageID"] = messageID
	}
	if actionID := strings.TrimSpace(record.ActionID); actionID != "" {
		meta["actionID"] = actionID
	}
	if answers := messageAnswersContext(record.Answers); len(answers) > 0 {
		meta["answers"] = answers
	}
	if resolved != nil {
		if requestTitle := strings.TrimSpace(resolved.Title); requestTitle != "" {
			meta["requestTitle"] = requestTitle
		}
	}
	ack := &op.ThreadControlAck{
		OK:             true,
		ThreadID:       threadID,
		OpCode:         op.OpThreadFollowUp,
		QueuedMessages: snapshot,
	}
	dispatch := &op.MessageReplyDispatch{
		Opcode:  op.OpThreadSubmit,
		Meta:    meta,
		Content: msg.Content,
	}
	return dispatch, ack, nil
}

func messageReplyQueueMessage(record op.MessageRecord, resolved *op.MessageRecord) op.Message {
	body := strings.TrimSpace(record.Body)
	actionID := strings.TrimSpace(record.ActionID)
	answers := normalizeMessageAnswers(record.Answers)
	var b strings.Builder
	if resolved != nil {
		if requestTitle := strings.TrimSpace(resolved.Title); requestTitle != "" {
			b.WriteString("User answered request: ")
			b.WriteString(requestTitle)
		}
	}
	if len(answers) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Answers:\n")
		for _, answer := range answers {
			questionText := messageQuestionText(resolved, answer.QuestionID)
			if questionText != "" {
				b.WriteString("- Question: ")
				b.WriteString(questionText)
				b.WriteString("\n")
			}
			if answer.Other {
				b.WriteString("  Answer: Other")
				if text := strings.TrimSpace(answer.Text); text != "" {
					b.WriteString(" — ")
					b.WriteString(text)
				}
				b.WriteString("\n")
				continue
			}
			if optionID := strings.TrimSpace(answer.OptionID); optionID != "" {
				b.WriteString("  Answer: ")
				b.WriteString(optionID)
				if label := strings.TrimSpace(answer.Label); label != "" && label != optionID {
					b.WriteString(" (")
					b.WriteString(label)
					b.WriteString(")")
				}
				b.WriteString("\n")
				continue
			}
			if display := strings.TrimSpace(messageAnswerDisplay(answer)); display != "" {
				b.WriteString("  Answer: ")
				b.WriteString(display)
				b.WriteString("\n")
			}
		}
	} else if body != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(body)
	}
	if actionID != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Selected action: ")
		b.WriteString(actionID)
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		text = "User replied in the message system."
	}
	return op.NewUserMessage(text)
}

func messageQuestionText(resolved *op.MessageRecord, questionID string) string {
	questionID = strings.TrimSpace(questionID)
	if questionID == "" || resolved == nil {
		return ""
	}
	for _, question := range resolved.Questions {
		if strings.TrimSpace(question.ID) == questionID {
			return strings.TrimSpace(question.Question)
		}
	}
	return ""
}

func messageReplySelectedSkillContext(record op.MessageRecord, resolved *op.MessageRecord) op.Meta {
	meta := op.Meta{
		"messageSystem": true,
		"messageID":     strings.TrimSpace(record.ID),
		"channelID":     strings.TrimSpace(record.ChannelID),
	}
	if replyTo := strings.TrimSpace(record.ReplyToMessageID); replyTo != "" {
		meta["replyToMessageID"] = replyTo
	}
	if actionID := strings.TrimSpace(record.ActionID); actionID != "" {
		meta["actionID"] = actionID
	}
	if answers := messageAnswersContext(record.Answers); len(answers) > 0 {
		meta["answers"] = answers
	}
	if resolved != nil {
		if requestTitle := strings.TrimSpace(resolved.Title); requestTitle != "" {
			meta["requestTitle"] = requestTitle
		}
	}
	return meta
}

func messageAnswerDisplay(answer op.MessageAnswer) string {
	label := strings.TrimSpace(answer.Label)
	optionID := strings.TrimSpace(answer.OptionID)
	text := strings.TrimSpace(answer.Text)
	if answer.Other {
		if label == "" {
			label = "Other"
		}
		if text != "" {
			return label + ": " + text
		}
		return label
	}
	if label == "" {
		label = optionID
	}
	if text != "" && label != "" {
		return label + "\nUser note: " + text
	}
	if text != "" {
		return text
	}
	return label
}

func messageAnswersText(answers []op.MessageAnswer) string {
	answers = normalizeMessageAnswers(answers)
	if len(answers) == 0 {
		return ""
	}
	lines := make([]string, 0, len(answers))
	for _, answer := range answers {
		if display := strings.TrimSpace(messageAnswerDisplay(answer)); display != "" {
			lines = append(lines, display)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func messageAnswersContext(answers []op.MessageAnswer) []any {
	answers = normalizeMessageAnswers(answers)
	if len(answers) == 0 {
		return nil
	}
	out := make([]any, 0, len(answers))
	for _, answer := range answers {
		item := map[string]any{"questionID": strings.TrimSpace(answer.QuestionID)}
		if optionID := strings.TrimSpace(answer.OptionID); optionID != "" {
			item["optionID"] = optionID
		}
		if label := strings.TrimSpace(answer.Label); label != "" {
			item["label"] = label
		}
		if answer.Other {
			item["other"] = true
		}
		if text := strings.TrimSpace(answer.Text); text != "" {
			item["text"] = text
		}
		out = append(out, item)
	}
	return out
}
