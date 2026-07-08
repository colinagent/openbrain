# 规范说明

### thread
会话统一用 thread 表示；聊天主提交流使用 `thread/submit`。

```
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "thread/submit",
  "params": "..."
}
```

兼容层仍可保留 `agent/call` / `agent/continue`，但它们不再是 thread chat 主协议。

### thread state query
```
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "thread/get",
  "params": "..."
}
```
### user
```
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "user/task/list",
  "params": "
}
```