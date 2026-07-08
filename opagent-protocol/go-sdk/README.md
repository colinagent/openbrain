# OpAgent Protocol Go SDK

Go SDK for Operation Agent Protocol (OpAgent Protocol), a framework for building multi-agent systems with interoperable agents, skills, tools, and subagents.

This SDK is adapted from and builds on the excellent [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk). We thank the Model Context Protocol project and its contributors for the foundational Go SDK design and implementation.

## Install

```bash
go get github.com/colinagent/openbrain/opagent-protocol/go-sdk
```

## Packages

- `op`: OpAgent protocol API for agents, skills, tools, subagents, sessions, and transports
- `auth`: auth helpers
- `jsonrpc`: JSON-RPC helpers

## Test

```bash
go test ./...
```
