// Copyright 2025 The Go MCP SDK Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// This file holds the request types.

package op

type (
	CallToolRequest                   = ServerRequest[*CallToolParamsRaw]
	CallAgentRequest                  = ServerRequest[*CallAgentParams]
	CompleteRequest                   = ServerRequest[*CompleteParams]
	GetPromptRequest                  = ServerRequest[*GetPromptParams]
	InitializedRequest                = ServerRequest[*InitializedParams]
	ListPromptsRequest                = ServerRequest[*ListPromptsParams]
	ListResourcesRequest              = ServerRequest[*ListResourcesParams]
	ListResourceTemplatesRequest      = ServerRequest[*ListResourceTemplatesParams]
	ListToolsRequest                  = ServerRequest[*ListToolsParams]
	ProgressNotificationServerRequest = ServerRequest[*ProgressNotificationParams]
	InfoNotificationServerRequest     = ServerRequest[*InfoNotificationParams]
	ReadResourceRequest               = ServerRequest[*ReadResourceParams]
	RootsListChangedRequest           = ServerRequest[*RootsListChangedParams]
	SubscribeRequest                  = ServerRequest[*SubscribeParams]
	UnsubscribeRequest                = ServerRequest[*UnsubscribeParams]
)

type (
	// OpAgentClientRequest               = ClientRequest[*OpAgentParams]
	OpAgentRequest                     = ClientRequest[*OpAgentParams]
	OpNodeRequest                      = ClientRequest[*OpNodeParams]
	CreateMessageRequest               = ClientRequest[*CreateMessageParams]
	ElicitRequest                      = ClientRequest[*ElicitParams]
	initializedClientRequest           = ClientRequest[*InitializedParams]
	InitializeRequest                  = ClientRequest[*InitializeParams]
	ListRootsRequest                   = ClientRequest[*ListRootsParams]
	LoggingMessageRequest              = ClientRequest[*LoggingMessageParams]
	ProgressNotificationClientRequest  = ClientRequest[*ProgressNotificationParams]
	InfoNotificationClientRequest      = ClientRequest[*InfoNotificationParams]
	PromptListChangedRequest           = ClientRequest[*PromptListChangedParams]
	ResourceListChangedRequest         = ClientRequest[*ResourceListChangedParams]
	ResourceUpdatedNotificationRequest = ClientRequest[*ResourceUpdatedNotificationParams]
	ToolListChangedRequest             = ClientRequest[*ToolListChangedParams]
)
