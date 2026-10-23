package linxapi

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Fake 是内存里的灵矽，供上层模块测试；行为对齐 HTTPClient 的约定。
type Fake struct {
	mu          sync.Mutex
	Agents      map[string]AgentSpec // id → spec
	Devices     map[string]string    // mac → agent id
	Memories    map[string]string    // agent id → memory id
	Attentions  map[string]string    // memory id + "/" + name → attention id
	Values      map[string]string    // memory id/mac/attention id → value
	Nodes       map[string][]string  // memory id/mac → contents
	Speakers    map[string]string    // speaker id → audio url
	Samples     map[string][]VoiceSample
	Bound       map[string][]string
	Messages    map[string][]Message // agent id → 一个会话的全部消息
	FailCreate  error                // 非 nil 时 CreateAgent 返回它
	FailPrereg  error
	SwitchCalls []string
	seq         int
}

func NewFake() *Fake {
	return &Fake{
		Agents: map[string]AgentSpec{}, Devices: map[string]string{}, Memories: map[string]string{},
		Attentions: map[string]string{}, Values: map[string]string{}, Nodes: map[string][]string{},
		Speakers: map[string]string{}, Samples: map[string][]VoiceSample{}, Bound: map[string][]string{},
		Messages: map[string][]Message{},
	}
}

var _ Client = (*Fake)(nil)

func (f *Fake) next(prefix string) string {
	f.seq++
	return fmt.Sprintf("%s%d", prefix, f.seq)
}

func (f *Fake) CreateAgent(_ context.Context, a AgentSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.FailCreate != nil {
		return "", f.FailCreate
	}
	id := f.next("agent-")
	f.Agents[id] = a
	return id, nil
}

func (f *Fake) UpdateAgent(_ context.Context, agentID string, a AgentSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.Agents[agentID]; !ok {
		return ErrNotFound
	}
	f.Agents[agentID] = a
	return nil
}

func (f *Fake) FindAgentByName(_ context.Context, name string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, a := range f.Agents {
		if a.Name == name {
			return id, true, nil
		}
	}
	return "", false, nil
}

func (f *Fake) PreregisterDevice(_ context.Context, agentID, mac string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.FailPrereg != nil {
		return f.FailPrereg
	}
	if _, ok := f.Devices[mac]; ok {
		return ErrAlreadyRegistered
	}
	f.Devices[mac] = agentID
	return nil
}

func (f *Fake) ActivateDevice(context.Context, string) error { return nil }

func (f *Fake) SwitchAgent(_ context.Context, mac, agentID string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.Devices[mac]; !ok {
		return ErrNotFound
	}
	f.Devices[mac] = agentID
	f.SwitchCalls = append(f.SwitchCalls, mac+"→"+agentID)
	return nil
}

func (f *Fake) EnableMemory(_ context.Context, agentID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m, ok := f.Memories[agentID]; ok {
		return m, nil
	}
	m := f.next("mem-")
	f.Memories[agentID] = m
	return m, nil
}

func (f *Fake) UpsertAttention(_ context.Context, memoryID, name, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := memoryID + "/" + name
	if a, ok := f.Attentions[k]; ok {
		return a, nil
	}
	a := f.next("att-")
	f.Attentions[k] = a
	return a, nil
}

func (f *Fake) SetAttentionValue(_ context.Context, memoryID, mac, attentionID, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Values[memoryID+"/"+mac+"/"+attentionID] = value
	return nil
}

func (f *Fake) AddMemoryNode(_ context.Context, memoryID, mac, content string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := memoryID + "/" + mac
	f.Nodes[k] = append(f.Nodes[k], content)
	return nil
}

func (f *Fake) CreateSpeaker(context.Context, string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.next("spk-"), nil
}

func (f *Fake) EnrollVoiceprint(_ context.Context, speakerID, audioURL string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Speakers[speakerID] = audioURL
	return nil
}

func (f *Fake) BindSpeakers(_ context.Context, agentID string, ids []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Bound[agentID] = ids
	return nil
}

func (f *Fake) SetOwnerOnly(context.Context, string, bool) error { return nil }

func (f *Fake) RecentVoiceSamples(_ context.Context, agentID string) ([]VoiceSample, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Samples[agentID], nil
}

func (f *Fake) Sessions(_ context.Context, agentID string, page, _ int) ([]Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if page > 1 || len(f.Messages[agentID]) == 0 {
		return nil, nil
	}
	return []Session{{ID: "s1", CreatedAt: f.Messages[agentID][0].At, ChatCount: len(f.Messages[agentID])}}, nil
}

func (f *Fake) SessionMessages(_ context.Context, agentID, sessionID string) ([]Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if sessionID != "s1" {
		return nil, ErrNotFound
	}
	return f.Messages[agentID], nil
}

// AgentOf 返回 mac 当前绑定的智能体，测试断言用。
func (f *Fake) AgentOf(mac string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Devices[strings.ToLower(mac)]
}
