# OpenBrain

[English README](./README.md)

> **构建你的第二大脑，连接其他人的大脑。**

OpenBrain 帮你把知识构建成一个持久的第二大脑，再连接个人、团队与公开大脑。
你的 Agents 可以按需查询你允许连接的大脑，而每一条连接都保持明确并由你控制。

[下载](https://openbrain.chat/download) · [官网](https://openbrain.chat) · [了解 Runtime](https://openbrain.chat/zh-CN/agent-os)

## 构建你的第二大脑

把笔记、本地工作区和知识源组织成你的第二大脑，让 Agent 在需要上下文时按需查询。

## 连接其他人的大脑

连接个人、团队与公开大脑，不必把所有知识压进同一个存储。Runtime 只在任务需要时，
在明确的访问与权限边界内查询已连接的大脑。

## The runtime your agents live on

Runtime 让第二大脑真正运行起来，为 Agents 提供使用知识、调用工具、协作和记忆所需的
上下文与能力：

- **Agents** — Coder、自定义 Agent、Memory Agent，以及受支持的知识 Agent。
- **Skills** — 按需加载的可复用 `SKILL.md` 工作流包。
- **Tools 与 MCP** — 内置工具和基于 `TOOL.md` 的 MCP Tool Servers。
- **Subagents 与 Memory** — 带持久 Thread 上下文的任务委派。
- **OpAgent Protocol** — 本地与远程 Runtime 使用同一套协议。
- **信任边界** — 权限配置、沙箱执行和操作前审批。

公开设计文档包括 [Runtime](docs/runtime.md)、[Skills](docs/skills.md)、
[Tools 与 MCP](docs/tools.md)、[Subagents](docs/subagent.md) 和
[Protocol](docs/opagent-protocol.md)。

## GBrain 支持

GBrain 是 OpenBrain 支持的知识 Agent 与 Brain 集成。需要它的知识工作流时可以挂载，
但它是可选集成，不是 OpenBrain 的核心身份或唯一基础。

## 开箱运行

下载 OpenBrain，构建你的第二大脑。Agent Runtime 可以在本地或远程工作区运行。

[下载 OpenBrain](https://openbrain.chat/download)

## 仓库结构

- `agents/`：内置产品 agent。`agents/coder` 是默认 coding agent；`agents/gbrain` 是基于 GBrain 的知识 agent。
- `tools/`：MCP 工具包。`tools/gbrain-cloud` 向 agent 暴露 OpenBrain Cloud GBrain MCP；shell/read/write/edit 是 runtime 内置能力。
- `opagent-runtime/`：公开 OpAgent runtime 包与入口。
- `opagent-protocol/`：公开 OpAgent 协议 SDK。
- `scripts/openbrain/`：通用 runtime/package 构建脚本。
- `docs/runtime.md`：runtime 设计。
- `docs/subagent.md`：subagent 设计。

OpenBrain Desktop 及其协作的本地 server 仍会以产品形式分发，但实现为
闭源代码，不属于本源码仓库。

## 开发

根目录 `go.work` 在本地链接 Go module；公开 module 文件不应包含本地路径 `replace`。

```bash
(cd opagent-runtime && go test ./...)
(cd opagent-protocol/go-sdk && go test ./...)
(cd agents/coder && go test ./...)
```

仓库根目录是 `go.work` workspace，本身不是 Go module，请在上述 module 目录中运行 Go 测试。

修改 runtime 或 subagent 前，请先阅读 [docs/runtime.md](docs/runtime.md) 和 [docs/subagent.md](docs/subagent.md)。

## 发布

即使源码闭源，OpenBrain Desktop 安装包仍继续通过 GitHub Releases 公开下载。每个产品 Release 主动上传的资产只包括 macOS、Linux、Windows 三个平台的 Desktop 安装包。Runtime 自更新 manifest、runtime bundle、bootstrap 二进制和桌面更新元数据由公开下载入口 `https://download.op-agent.com` 提供。

## 许可证

本仓库使用多种许可证：

- **AGPL-3.0** — 本仓库中的源码：`agents/`、`tools/`、`opagent-runtime/`、`opagent-protocol/`、`skills/`、`docs/`，以及 `scripts/openbrain/` 下的通用辅助脚本。见 [LICENSE](LICENSE) 与 [NOTICE](NOTICE)。
- **专有软件** — OpenBrain Desktop 与本地 server 独立分发，其源码不由本仓库授权。
- **MIT** — GBrain 本身是外部 `garrytan/gbrain` 项目。OpenBrain release 辅助脚本直接构建上游源码，或使用与上游完全一致的 `colinagent/gbrain` 二进制镜像。

如果你修改或分发 OpenBrain 代码，必须遵守 AGPL-3.0（包括在要求时提供源码），并按 NOTICE 保留 OpenBrain 署名。版权归 OpAgent Inc. 所有。
