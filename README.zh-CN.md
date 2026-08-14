# OpenBrain

[English README](README.md)

> **构建你的第二大脑，连接其他人的大脑。**

OpenBrain 帮你把知识构建成一个持久的第二大脑，再连接个人、团队与公开 Brain。
你的 Agent 可以按需查询你允许连接的 Brain，而每一条连接都保持明确并由你控制。

[下载](https://download.op-agent.com) · [官网](https://openbrain.chat) · [Agent OS](https://openbrain.chat/zh-CN/agent-os)

## 构建你的第二大脑

把笔记、本地工作区和知识源组织成 Agent 在需要上下文时可以查询的 Brain。源材料和
长期知识保持可见，不会随一次聊天消失。

## 连接其他人的大脑

连接个人、团队与公开 Brain，不必把所有知识压进同一个存储。Runtime 只在任务需要时，
在明确的账户、组织、工作区、共享与权限边界内查询已连接的 Brain。

## Agent 真正生活的 Runtime

Runtime 让第二大脑真正运行起来，为 Agent 提供使用知识、调用工具、协作和记忆所需的
上下文与能力：

- **Agents** — 自定义、Coding、Memory 以及受支持的知识 Agent。
- **Skills** — 按需加载的可复用 `SKILL.md` 工作流包。
- **Tools 与 MCP** — 内置 Tool 和基于 `TOOL.md` 的 MCP Tool Server。
- **Subagents 与 Memory** — 带持久 Thread 上下文的任务委派。
- **信任边界** — 权限配置、沙箱执行和操作前审批。

## Powered by OpAgent

OpenBrain 基于 [OpAgent](https://github.com/colinagent/opagent) 运行。OpAgent
是采用 Apache-2.0 许可证的 Go Agent 框架与多 Agent OS Runtime，提供可移植
Runtime、Protocol SDK、沙箱、权限、Agent/Tool/Skill 模型、Subagent 和文件原生线程引擎。
本仓库只包含 OpenBrain 的产品文档与产品专属集成，不再保存 Runtime 或 Protocol 核心副本。

OpenBrain 在此基础上增加：

- 基于 GBrain 的知识 Agent 与专属 Skill pack。
- OpenBrain Cloud GBrain 与 Cloud Sync 集成。
- 产品 Session、审批、Prompt、Schedule 和托管服务策略。
- 面向产品客户端与 Relay 的公开 Remote Control wire contract。
- 在同一 Desktop 体验中统一本地与远程工作区。

GBrain 是可选的知识 Agent 集成，而不是 OpenBrain 的唯一基础；需要它的专注知识工作流时
再进行挂载。

公开文档包括 [Runtime 集成](docs/runtime.md)、[产品沙箱策略](docs/sandbox.md)、
[Skills](docs/skills.md)、[Tools](docs/tools.md)、[Subagents](docs/subagent.md) 和
[产品协议](docs/opagent-protocol.md)。

## 仓库结构

- `agents/gbrain/`：OpenBrain 的 GBrain 知识 Agent 与产品 Skill pack。
- `skills/openbrain-cloud-sync/`：产品 Cloud workspace 同步。
- `tools/gbrain-cloud/`：使用产品 Session 认证的 MCP endpoint manifest。
- `integrations/opagent/`：OpenBrain 提供给公共 OpAgent Runtime 的宿主钩子。
- `protocol/remotecontrol/`：版本化 OpenBrain Remote Control wire contract。
- `docs/`：公开产品行为和集成文档。

OpenBrain Desktop、iOS、云服务以及协作的本地 `openbrain-server` 均为专有软件；
它们的源码、构建签名流程、部署拓扑和运维文档不会进入本仓库。

## 开发

仓库根目录是 Go workspace，不是 Go module。OpAgent 依赖来自
`github.com/colinagent/opagent`，不会复制或 vendor 到本仓库。

```bash
(cd integrations/opagent && go test ./...)
(cd protocol/remotecontrol && go test ./...)
(cd skills/openbrain-cloud-sync && go test ./...)
node --test skills/openbrain-cloud-sync/SKILL.source.test.mjs
scripts/check-openbrain-public-boundary.sh
```

## 许可证

本仓库中的 OpenBrain 源码采用 AGPL-3.0，详见 [LICENSE](LICENSE) 与
[NOTICE](NOTICE)。OpAgent 是独立的 Apache-2.0 依赖；GBrain 独立采用 MIT 许可证。

版权所有 © 2026 OpAgent Inc.
