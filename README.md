# tg_chat

一个轻量 Telegram 群聊 Bot，面向低配机器（1C1G Alpine）设计：
- 仅在指定群组中响应。
- 仅在被 @ 或命中特定触发词时响应。
- 仅管理员可私聊配置，非管理员不响应。
- 支持 OpenAI-Compatible 接口。
- 配置变更采用“保存后重启生效”模式。
- 单群组限流：每分钟最多 5 次模型调用（默认）。
- 上下文达到估算 64k tokens 时自动总结并切换到新会话（默认）。

## 目录结构

- `cmd/bot/main.go`：程序入口。
- `internal/config`：配置加载/保存/校验。
- `internal/agent`：模型调用客户端。
- `agents/default/`：默认 agent（系统提示词）目录，后续可自行新增。
- `config.example.json`：配置示例。

## 快速开始

1. 安装 Go（建议 1.22+）。
2. 复制配置并修改：

```bash
cp config.example.json config.json
```

3. 启动：

```bash
go run ./cmd/bot -config config.json
```

## 管理员私聊命令

> 仅配置中的 `admin_uids` 可以使用。

- `/help` 查看命令列表
- `/show` 查看当前配置（API Key 会脱敏）
- `/set_group <id>` 设置群组 ID
- `/set_admins <uid1,uid2>` 设置管理员 UID 列表
- `/set_trigger <word>` 设置触发词
- `/set_model <model>` 设置模型名
- `/set_base_url <url>` 设置接口地址（OpenAI-Compatible）
- `/set_api_key <key>` 设置密钥
- `/set_agent_file <path>` 设置 agent 提示词文件路径
- `/save` 将当前内存配置写入配置文件（提示“重启后生效”）
- `/restart_now` 立即退出进程（配合 systemd/supervisor/docker restart 自动拉起）

## 响应规则

机器人只会在以下条件都满足时回复：
1. 消息来自 `allowed_group_id` 指定群组；
2. 消息包含 `@botusername` 或包含 `trigger_word`。

## 生产部署建议（Alpine）

建议使用进程守护，保证 `/restart_now` 后自动拉起：
- systemd
- supervisor
- Docker `--restart=always`

## 新增 agent

把新提示词放到 `agents/<name>/system_prompt.md`，然后管理员私聊执行：

```text
/set_agent_file agents/<name>/system_prompt.md
/save
/restart_now
```


## 一键查看部署命令

详见 `docs/setup_alpine.md`（包含 Alpine 安装、构建、OpenRC 托管、更新重启命令）。

## 上下文与限流默认策略

- 限流范围：按群组统计。
- 限流阈值：每分钟最多 5 次模型调用。
- 上下文触发阈值：估算 64k tokens。
- 触发后动作：自动调用摘要流程，生成会话摘要并清空历史明细，仅保留摘要继续后续对话。
