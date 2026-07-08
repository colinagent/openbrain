package core

import (
	"encoding/json"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func OpThreadReviewListHandler(req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	var params op.ThreadReviewListParams
	if err := decodeJSONContent(req.Params.Content, &params); err != nil {
		return nil, err
	}
	reviews, err := listThreadReviewStates(params)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(op.ThreadReviewListResult{Reviews: reviews})
	if err != nil {
		return nil, err
	}
	return &op.OpNodeResult{
		Content: &op.JsonContent{Raw: raw},
		Meta:    req.Params.Meta,
	}, nil
}

func OpThreadReviewResolveHandler(req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	var params op.ThreadReviewResolveParams
	if err := decodeJSONContent(req.Params.Content, &params); err != nil {
		return nil, err
	}
	review, err := resolveThreadReview(params)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(op.ThreadReviewResolveResult{Review: review})
	if err != nil {
		return nil, err
	}
	return &op.OpNodeResult{
		Content: &op.JsonContent{Raw: raw},
		Meta:    req.Params.Meta,
	}, nil
}

func OpThreadReviewRollbackHandler(req *op.OpNodeRequest) (*op.OpNodeResult, error) {
	var params op.ThreadReviewRollbackParams
	if err := decodeJSONContent(req.Params.Content, &params); err != nil {
		return nil, err
	}
	review, err := rollbackThreadReview(params)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(op.ThreadReviewRollbackResult{Review: review})
	if err != nil {
		return nil, err
	}
	return &op.OpNodeResult{
		Content: &op.JsonContent{Raw: raw},
		Meta:    req.Params.Meta,
	}, nil
}
