# OpAgent

[English](README.md) | 简体中文

OpAgent 是 OpenBrain 使用的 AgentOS runtime。

操作系统连接你和应用，OpAgent 连接你和 AI Agent。一切皆文件，Agent
生活在你的目录里。

OpAgent 围绕一个可见、文件原生的 Agent 工作区构建：对话、工具、技能、知识和文件变更都留在工作区里，而不是藏在聊天框后面。

## 产品功能

### Agent 生活在你的目录里

Agent 贴近它需要处理的文件、工具和上下文。目录可以拥有自己的 Agent 配置，而不是依赖一个和工作区脱节的聊天框。

![OpAgent Agent 生活在你的目录里](assets/readme/agents-live-in-folder.gif)

### Agent 改了什么，一目了然

每次文件修改都会以清晰的 diff 呈现。接受、编辑、回退，由你决定，工作区里最终落下什么始终可控。

![OpAgent Agent 文件 diff 审阅](https://www.openbrain.io/website/agent-file-diff-review.png)

### Agent 的一切，可见、可管理

对话、工具、技能、知识全部聚合在同一个工作区。Agent 环境可以被检查、管理，而不是隐藏在聊天框后面。

![OpAgent 工作区里的 agents、skills、tools 和 marketplace](https://www.openbrain.io/website/workspace-agents-visible-manageable.png)

### 对话不再是一次性的

每段对话都会沉淀为 Markdown 文件，可以编辑、搜索、版本管理和复用，而不是埋没在聊天记录里。

![OpAgent 对话保存为 Markdown](https://www.openbrain.io/website/conversations-that-dont-disappear-workspace.png)

### 一套 Agent 环境，本地远程通用

模型配置随你走，Agent 按需安装。远程工作不再需要在每台机器上重新搭一遍环境。

![OpAgent 远程工作区演示](assets/readme/remote-workspace.gif)

### Markdown 就是界面

实时预览、表格、Mermaid 图表、frontmatter 和源码编辑保持在一起。文档既是交互界面，也是可长期保存的记录。

![OpAgent Markdown 界面](https://www.openbrain.io/website/markdown-interface.png)

## 这个仓库

这个目录包含 OpAgent 的开源 runtime 代码，可用于构建和测试本地 OpenBrain runtime 进程。

私有部署脚本、密钥、托管服务内部实现和仅用于内部发布的资产不属于这里。

## 仓库结构

- `cmd/opagent-runtime`: runtime 入口。
- `cmd/opagent-bootstrap`: bootstrap 和 updater 入口。
- `internal`: runtime 实现。
- `packages/ai`: canonical AI history、provider adapter、replay helper 和流式支持。
- `../docs/runtime.md`: 公开 runtime 设计。

## 构建

```bash
go build ./cmd/opagent-runtime
```

## 测试

```bash
go test ./...
```

检查 TypeScript 协议 SDK：

```bash
cd ../opagent-protocol/ts-sdk
npm ci
npm run check
```

## 链接

- 官网: <https://www.openbrain.io>
- 文档: <https://docs.openbrain.io>
- Runtime 文档: [../docs/runtime.md](../docs/runtime.md)
- Subagent 文档: [../docs/subagent.md](../docs/subagent.md)
- OpAgent Protocol spec: [../opagent-protocol/spec.md](../opagent-protocol/spec.md)

## License

AGPL-3.0。详见 [LICENSE](LICENSE) 与仓库根目录 [NOTICE](../NOTICE)。
