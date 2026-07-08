package core

import (
	"encoding/json"
	"fmt"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/ai"
)

func decodeJSONContent[T any](content op.Content, out *T) error {
	jsonContent, ok := content.(*op.JsonContent)
	if !ok {
		return fmt.Errorf("content must be json")
	}
	if out == nil {
		return fmt.Errorf("output must not be nil")
	}
	return json.Unmarshal(jsonContent.Raw, out)
}

func OpThreadCreateHandler(req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	var params op.ThreadCreateParams
	if err := decodeJSONContent(req.Params.Content, &params); err != nil {
		return nil, err
	}
	result, err := createThread(params)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &op.OpNodeResult{
		Content: &op.JsonContent{Raw: raw},
		Meta:    req.Params.Meta,
	}, nil
}

func OpThreadForkHandler(req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	var params op.ThreadForkParams
	if err := decodeJSONContent(req.Params.Content, &params); err != nil {
		return nil, err
	}
	meta, err := forkThread(params)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	return &op.OpNodeResult{
		Content: &op.JsonContent{Raw: raw},
		Meta:    req.Params.Meta,
	}, nil
}

func OpThreadMetaGetHandler(req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	var query op.ThreadMetaQuery
	if err := decodeJSONContent(req.Params.Content, &query); err != nil {
		return nil, err
	}
	meta, err := defaultThreadStore.GetMeta(query)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	return &op.OpNodeResult{
		Content: &op.JsonContent{Raw: raw},
		Meta:    req.Params.Meta,
	}, nil
}

func OpThreadMetaUpdateHandler(req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	var params op.ThreadMetaUpdateParams
	if err := decodeJSONContent(req.Params.Content, &params); err != nil {
		return nil, err
	}
	meta, err := updateThreadMeta(params)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	return &op.OpNodeResult{
		Content: &op.JsonContent{Raw: raw},
		Meta:    req.Params.Meta,
	}, nil
}

func OpThreadSnapshotGetHandler(req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	var query op.ThreadMetaQuery
	if err := decodeJSONContent(req.Params.Content, &query); err != nil {
		return nil, err
	}
	snapshot, err := getThreadSnapshotWithMeta(query, req.Params.Meta)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		snapshot = &ai.ThreadSnapshot{}
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	return &op.OpNodeResult{
		Content: &op.JsonContent{Raw: raw},
		Meta:    req.Params.Meta,
	}, nil
}
