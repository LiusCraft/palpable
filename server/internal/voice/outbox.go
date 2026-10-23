package voice

import (
	"sync"

	"github.com/LiusCraft/palpable/server/internal/id"
	"github.com/LiusCraft/palpable/server/internal/protocol"
)

// outbox 是一个听者的有序下行队列：控制消息与音频帧保持先后顺序，
// 音频最多留 queueFrames 帧，满了丢最旧的一帧；控制消息从不丢弃。
// 由唯一的写 goroutine 发出，一个慢听者不会拖慢其他人。
type outbox struct {
	p    id.ParticipantID
	sink Sink

	mu     sync.Mutex
	items  []item
	frames int
	closed bool
	notify chan struct{}
	done   chan struct{}
}

type item struct {
	msg   protocol.Message
	frame []byte
}

func newOutbox(p id.ParticipantID, sink Sink) *outbox {
	o := &outbox{p: p, sink: sink, notify: make(chan struct{}, 1), done: make(chan struct{})}
	go o.run()
	return o
}

func (o *outbox) pushFrame(f []byte) {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return
	}
	if o.frames >= queueFrames {
		for i, it := range o.items {
			if it.msg == nil {
				o.items = append(o.items[:i], o.items[i+1:]...)
				o.frames--
				break
			}
		}
	}
	o.items = append(o.items, item{frame: f})
	o.frames++
	o.mu.Unlock()
	o.wake()
}

func (o *outbox) pushMsg(m protocol.Message) {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return
	}
	o.items = append(o.items, item{msg: m})
	o.mu.Unlock()
	o.wake()
}

func (o *outbox) wake() {
	select {
	case o.notify <- struct{}{}:
	default:
	}
}

func (o *outbox) close() {
	o.mu.Lock()
	o.closed = true
	o.items = nil
	o.mu.Unlock()
	o.wake()
	<-o.done
}

func (o *outbox) run() {
	defer close(o.done)
	for range o.notify {
		for {
			o.mu.Lock()
			if o.closed {
				o.mu.Unlock()
				return
			}
			if len(o.items) == 0 {
				o.mu.Unlock()
				break
			}
			it := o.items[0]
			o.items = o.items[1:]
			if it.msg == nil {
				o.frames--
			}
			o.mu.Unlock()

			if it.msg != nil {
				_ = o.sink.Send(o.p, it.msg)
			} else {
				_ = o.sink.SendAudio(o.p, it.frame)
			}
		}
	}
}
