package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"tg_chat/internal/agent"
	"tg_chat/internal/config"
	"tg_chat/internal/session"
	"tg_chat/internal/telegram"
)

const (
	defaultTokenThreshold = 64000
	defaultMaxCallsPerMin = 5
	defaultTailMessages   = 12
)

func main() {
	configPath := flag.String("config", "config.json", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	agentClient, err := buildAgent(cfg)
	if err != nil {
		log.Fatalf("init agent failed: %v", err)
	}
	mgr := session.NewManager(defaultTokenThreshold, defaultMaxCallsPerMin, defaultTailMessages)

	tg := telegram.New(cfg.BotToken)
	if err := tg.GetMe(context.Background()); err != nil {
		log.Fatalf("telegram getMe failed: %v", err)
	}

	var pendingCfg *config.Config
	var offset int64
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
		updates, err := tg.GetUpdates(ctx, offset, 60)
		cancel()
		if err != nil {
			log.Printf("get updates error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, update := range updates {
			offset = update.UpdateID + 1
			if update.Message == nil {
				continue
			}
			msg := update.Message
			text := strings.TrimSpace(msg.Text)

			if msg.Chat.IsPrivate() {
				if !cfg.IsAdmin(msg.From.ID) {
					continue
				}
				nextCfg, err := handleAdminPrivate(tg, msg.Chat.ID, text, cfg, pendingCfg, *configPath)
				if err != nil {
					log.Printf("private cmd error: %v", err)
					continue
				}
				if nextCfg != nil {
					pendingCfg = nextCfg
					cfg = *nextCfg
					agentClient, err = buildAgent(cfg)
					if err != nil {
						log.Printf("reload agent failed: %v", err)
					}
				}
				continue
			}

			if msg.Chat.ID != cfg.AllowedGroupID {
				continue
			}
			if !shouldReply(text, cfg.TriggerWord, tg.Username()) {
				continue
			}
			allowed, retryAfter := mgr.AllowCall(msg.Chat.ID, time.Now())
			if !allowed {
				_ = tg.SendMessage(context.Background(), msg.Chat.ID, fmt.Sprintf("调用过于频繁，请 %.0f 秒后再试。", retryAfter.Seconds()+1))
				continue
			}

			if mgr.NeedsSummarize(msg.Chat.ID) {
				sInput := mgr.SummarizeInput(msg.Chat.ID)
				sCtx, sCancel := context.WithTimeout(context.Background(), 55*time.Second)
				summary, sErr := agentClient.Summarize(sCtx, sInput)
				sCancel()
				if sErr == nil {
					mgr.ApplySummary(msg.Chat.ID, summary)
				}
			}

			clean := sanitizeInput(text, cfg.TriggerWord, tg.Username())
			if clean == "" {
				continue
			}
			contextMsgs := mgr.BuildContext(msg.Chat.ID, clean)
			ctxReply, cancelReply := context.WithTimeout(context.Background(), 55*time.Second)
			reply, err := agentClient.Reply(ctxReply, contextMsgs)
			cancelReply()
			if err != nil {
				log.Printf("model call failed: %v", err)
				reply = "模型调用失败，请稍后再试。"
			}
			mgr.RecordExchange(msg.Chat.ID, clean, reply)
			_ = tg.SendMessage(context.Background(), msg.Chat.ID, reply)
		}
	}
}

func buildAgent(cfg config.Config) (*agent.Client, error) {
	systemPrompt, err := agent.LoadSystemPrompt(cfg.AgentFile)
	if err != nil {
		return nil, err
	}
	return agent.New(cfg.Model, systemPrompt), nil
}

func shouldReply(text, triggerWord, botUsername string) bool {
	low := strings.ToLower(text)
	if strings.Contains(low, strings.ToLower(triggerWord)) {
		return true
	}
	if botUsername == "" {
		return false
	}
	return strings.Contains(low, "@"+strings.ToLower(botUsername))
}

func sanitizeInput(text, triggerWord, botUsername string) string {
	out := strings.ReplaceAll(text, triggerWord, "")
	if botUsername != "" {
		mention := "@" + botUsername
		out = strings.ReplaceAll(out, mention, "")
		out = strings.ReplaceAll(out, strings.ToLower(mention), "")
	}
	return strings.TrimSpace(out)
}

func handleAdminPrivate(tg *telegram.API, chatID int64, text string, current config.Config, pending *config.Config, path string) (*config.Config, error) {
	active := current
	if pending != nil {
		active = *pending
	}

	send := func(s string) error {
		return tg.SendMessage(context.Background(), chatID, s)
	}

	switch {
	case strings.HasPrefix(text, "/help"):
		return pending, send("/show\n/set_group <id>\n/set_admins <uid1,uid2>\n/set_trigger <word>\n/set_model <model>\n/set_base_url <url>\n/set_api_key <key>\n/set_agent_file <path>\n/save\n/restart_now")
	case text == "/show":
		safe := active
		safe.Model.APIKey = "***"
		return pending, send(fmt.Sprintf("当前配置: %+v", safe))
	case text == "/save":
		if pending == nil {
			return pending, send("没有待保存配置。")
		}
		if err := config.Save(path, *pending); err != nil {
			return pending, send("保存失败: " + err.Error())
		}
		return pending, send("配置已保存。重启后生效；如果要立即生效，请发送 /restart_now")
	case text == "/restart_now":
		_ = send("收到重启指令，进程将退出，请由 systemd/supervisor/docker 拉起。")
		os.Exit(0)
	}

	parts := strings.SplitN(text, " ", 2)
	if len(parts) != 2 {
		return pending, send("未识别命令，输入 /help 查看帮助。")
	}
	cmd, val := parts[0], strings.TrimSpace(parts[1])
	set := active

	switch cmd {
	case "/set_group":
		id, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return pending, send("group id 格式错误")
		}
		set.AllowedGroupID = id
	case "/set_admins":
		items := strings.Split(val, ",")
		admins := make([]int64, 0, len(items))
		for _, it := range items {
			uid, err := strconv.ParseInt(strings.TrimSpace(it), 10, 64)
			if err != nil {
				return pending, send("admin uid 列表格式错误")
			}
			admins = append(admins, uid)
		}
		set.AdminUIDs = admins
	case "/set_trigger":
		set.TriggerWord = val
	case "/set_model":
		set.Model.Model = val
	case "/set_base_url":
		set.Model.BaseURL = val
	case "/set_api_key":
		set.Model.APIKey = val
	case "/set_agent_file":
		set.AgentFile = val
	default:
		return pending, send("未识别命令，输入 /help 查看帮助。")
	}

	if err := set.Validate(); err != nil {
		return pending, send("配置校验失败: " + err.Error())
	}
	if err := send("内存配置已更新。执行 /save 保存；保存后重启生效，可执行 /restart_now"); err != nil {
		return pending, err
	}
	return &set, nil
}
