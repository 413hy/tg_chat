package session

import (
	"fmt"
	"sync"
	"time"
)

type Message struct {
	Role    string
	Content string
}

type Conversation struct {
	SessionID       int64
	Summary         string
	Messages        []Message
	EstimatedTokens int
	CallTimestamps  []time.Time
}

type Manager struct {
	mu              sync.Mutex
	groups          map[int64]*Conversation
	TokenThreshold  int
	MaxCallsPerMin  int
	TailMsgKeep     int
	nextSessionSeed int64
}

func NewManager(tokenThreshold, maxCallsPerMin, tailMsgKeep int) *Manager {
	if tokenThreshold <= 0 {
		tokenThreshold = 64000
	}
	if maxCallsPerMin <= 0 {
		maxCallsPerMin = 5
	}
	if tailMsgKeep <= 0 {
		tailMsgKeep = 12
	}
	return &Manager{
		groups:          map[int64]*Conversation{},
		TokenThreshold:  tokenThreshold,
		MaxCallsPerMin:  maxCallsPerMin,
		TailMsgKeep:     tailMsgKeep,
		nextSessionSeed: time.Now().Unix(),
	}
}

func (m *Manager) get(groupID int64) *Conversation {
	if c, ok := m.groups[groupID]; ok {
		return c
	}
	m.nextSessionSeed++
	c := &Conversation{SessionID: m.nextSessionSeed}
	m.groups[groupID] = c
	return c
}

func (m *Manager) AllowCall(groupID int64, now time.Time) (bool, time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.get(groupID)
	windowStart := now.Add(-1 * time.Minute)
	kept := c.CallTimestamps[:0]
	for _, t := range c.CallTimestamps {
		if t.After(windowStart) {
			kept = append(kept, t)
		}
	}
	c.CallTimestamps = kept
	if len(c.CallTimestamps) >= m.MaxCallsPerMin {
		retry := c.CallTimestamps[0].Add(time.Minute).Sub(now)
		if retry < 0 {
			retry = 0
		}
		return false, retry
	}
	c.CallTimestamps = append(c.CallTimestamps, now)
	return true, 0
}

func (m *Manager) NeedsSummarize(groupID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.get(groupID)
	return c.EstimatedTokens >= m.TokenThreshold
}

func (m *Manager) SummarizeInput(groupID int64) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.get(groupID)
	text := ""
	if c.Summary != "" {
		text += "已有摘要:\n" + c.Summary + "\n\n"
	}
	text += "请基于以下对话生成新的压缩摘要:\n"
	for _, msg := range c.Messages {
		text += fmt.Sprintf("[%s] %s\n", msg.Role, msg.Content)
	}
	return text
}

func (m *Manager) ApplySummary(groupID int64, summary string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.get(groupID)
	c.Summary = summary
	c.Messages = nil
	c.EstimatedTokens = estimateTokens(summary)
	m.nextSessionSeed++
	c.SessionID = m.nextSessionSeed
}

func (m *Manager) BuildContext(groupID int64, userText string) []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.get(groupID)
	ctx := make([]Message, 0, 1+len(c.Messages)+1)
	if c.Summary != "" {
		ctx = append(ctx, Message{Role: "system", Content: "会话摘要（请作为上下文遵循）: " + c.Summary})
	}
	start := 0
	if len(c.Messages) > m.TailMsgKeep {
		start = len(c.Messages) - m.TailMsgKeep
	}
	ctx = append(ctx, c.Messages[start:]...)
	ctx = append(ctx, Message{Role: "user", Content: userText})
	return ctx
}

func (m *Manager) RecordExchange(groupID int64, userText, reply string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.get(groupID)
	c.Messages = append(c.Messages,
		Message{Role: "user", Content: userText},
		Message{Role: "assistant", Content: reply},
	)
	c.EstimatedTokens += estimateTokens(userText) + estimateTokens(reply)
}

func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return len([]rune(s)) * 2
}
