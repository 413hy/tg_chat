# Alpine 1C1G 部署命令（手动）

> 适用于你自己在服务器上一步步执行。

## 1) 安装依赖

```bash
apk update
apk add --no-cache git go ca-certificates tzdata
update-ca-certificates
```

## 2) 拉代码并准备配置

```bash
cd /opt
git clone -b codex/clarify-project-goals-and-objectives https://github.com/413hy/tg_chat.git
cd tg_chat
cp config.example.json config.json
```

编辑配置：

```bash
vi config.json
```

至少改这些字段：
- `bot_token`
- `allowed_group_id`
- `admin_uids`
- `trigger_word`
- `model.base_url`
- `model.api_key`
- `model.model`

## 3) 构建

```bash
cd /opt/tg_chat
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/tg_chat ./cmd/bot
```

## 4) 前台试运行

```bash
cd /opt/tg_chat
./bin/tg_chat -config ./config.json
```

## 5) 使用 OpenRC 后台托管（Alpine 默认）

创建服务脚本：

```bash
cat >/etc/init.d/tg_chat <<'SH'
#!/sbin/openrc-run
name="tg_chat"
description="Telegram group chat bot"

command="/opt/tg_chat/bin/tg_chat"
command_args="-config /opt/tg_chat/config.json"
command_user="root:root"

directory="/opt/tg_chat"
pidfile="/run/${RC_SVCNAME}.pid"
command_background="yes"

output_log="/var/log/tg_chat.log"
error_log="/var/log/tg_chat.err"

depend() {
  need net
}
SH
chmod +x /etc/init.d/tg_chat
```

设置开机自启并启动：

```bash
rc-update add tg_chat default
rc-service tg_chat start
rc-service tg_chat status
```

查看日志：

```bash
tail -f /var/log/tg_chat.log /var/log/tg_chat.err
```

## 6) 更新发布流程

```bash
cd /opt/tg_chat
git pull
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/tg_chat ./cmd/bot
rc-service tg_chat restart
```
