package voice

import (
	"log/slog"
	"sync"

	"github.com/kazzmir/opus-go/opus"

	"github.com/LiusCraft/palpable/server/internal/id"
)

const (
	mixRate    = 16000
	mixFrame   = mixRate * 60 / 1000 // 每个混音帧 60ms，与设备约定的 peer 音频一致
	mixBitrate = 24000
	maxPacket  = 4 << 10
	// 单路最多缓存 3 帧：设备与服务端时钟有漂移时丢最旧的，延迟不会越积越大。
	maxQueued = 3 * mixFrame
)

// chanMix 是一个频道在"多人同时说话"时的混音状态。只有一个人说话时不会创建它，
// 音频仍然原样透传 Opus；第二个人开口时 voice 才为该频道建一个。
// 解码与编码都在 chanMix 自己的锁里做，不占用 Router 的全局锁。
type chanMix struct {
	mu     sync.Mutex
	closed bool
	src    map[id.ParticipantID]*mixSource
	enc    map[id.DeviceID]*opus.Encoder
	pcm    []int16 // 解码用的临时缓冲
}

type mixSource struct {
	dec *opus.Decoder
	q   []int16
}

// mixTarget 是一个听者这一拍要听到的发言者，已排除其自己的话与不该听的人。
type mixTarget struct {
	listener id.DeviceID
	out      *outbox
	from     []id.ParticipantID
}

func newChanMix() *chanMix {
	return &chanMix{
		src: map[id.ParticipantID]*mixSource{},
		enc: map[id.DeviceID]*opus.Encoder{},
		pcm: make([]int16, 2*mixFrame),
	}
}

// push 解码一帧发言者的 Opus 放进他的队列。设备给的帧不可信：解码库在个别坏包上可能 panic，
// 这里兜住，坏帧直接丢弃。采样率统一解到 16kHz，帧长由包自己带（20/40/60ms 都行），
// 所以灵矽 24kHz 的 TTS 也能混。
func (m *chanMix) push(sp id.ParticipantID, frame []byte) {
	if len(frame) == 0 || len(frame) > maxPacket {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	s := m.src[sp]
	if s == nil {
		dec, err := opus.NewDecoder(mixRate, 1)
		if err != nil {
			slog.Warn("mix decoder", "error", err)
			return
		}
		s = &mixSource{dec: dec}
		m.src[sp] = s
	}
	n, ok := m.decode(s.dec, frame)
	if !ok {
		return
	}
	s.q = append(s.q, m.pcm[:n]...)
	if over := len(s.q) - maxQueued; over > 0 {
		s.q = append(s.q[:0], s.q[over:]...)
	}
}

func (m *chanMix) decode(dec *opus.Decoder, frame []byte) (n int, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			slog.Debug("mix decode panic", "panic", r)
			n, ok = 0, false
		}
	}()
	n, err := dec.Decode(frame, m.pcm, len(m.pcm), false)
	return n, err == nil && n > 0
}

// tick 取出每个发言者的一帧，给每个听者混一份并编码下发。sources 之外的发言者与 targets
// 之外的听者的编解码器在这里释放。没有人真的出声（队列都空）的一拍不发帧。
func (m *chanMix) tick(sources []id.ParticipantID, targets []mixTarget) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	live := make(map[id.ParticipantID]bool, len(sources))
	for _, sp := range sources {
		live[sp] = true
	}
	for sp, s := range m.src {
		if !live[sp] {
			_ = s.dec.Close()
			delete(m.src, sp)
		}
	}
	pulled := make(map[id.ParticipantID][]int16, len(m.src))
	for sp, s := range m.src {
		if len(s.q) >= mixFrame {
			pulled[sp] = append([]int16(nil), s.q[:mixFrame]...)
			s.q = append(s.q[:0], s.q[mixFrame:]...)
		}
	}

	heard := make(map[id.DeviceID]bool, len(targets))
	for _, t := range targets {
		heard[t.listener] = true
		var acc [mixFrame]int32
		any := false
		for _, sp := range t.from {
			fr := pulled[sp]
			if fr == nil {
				continue
			}
			any = true
			for i, v := range fr {
				acc[i] += int32(v)
			}
		}
		if !any {
			continue
		}
		mixed := make([]int16, mixFrame)
		for i, v := range acc {
			mixed[i] = int16(max(-32768, min(32767, v)))
		}
		pkt, ok := m.encode(t.listener, mixed)
		if !ok {
			continue
		}
		t.out.pushFrame(pkt)
	}
	for d, e := range m.enc {
		if !heard[d] {
			_ = e.Close()
			delete(m.enc, d)
		}
	}
}

func (m *chanMix) encode(d id.DeviceID, pcm []int16) (pkt []byte, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			slog.Debug("mix encode panic", "panic", r)
			pkt, ok = nil, false
		}
	}()
	e := m.enc[d]
	if e == nil {
		var err error
		if e, err = opus.NewEncoder(mixRate, 1, opus.ApplicationVoIP); err != nil {
			slog.Warn("mix encoder", "error", err)
			return nil, false
		}
		_ = e.SetBitrate(mixBitrate)
		_ = e.SetComplexity(5)
		m.enc[d] = e
	}
	buf := make([]byte, maxPacket)
	n, err := e.Encode(pcm, mixFrame, buf)
	if err != nil || n <= 0 {
		return nil, false
	}
	return buf[:n:n], true
}

// close 释放全部编解码器；之后 push 与 tick 都是空操作。
func (m *chanMix) close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for _, s := range m.src {
		_ = s.dec.Close()
	}
	for _, e := range m.enc {
		_ = e.Close()
	}
	m.src, m.enc = nil, nil
}
