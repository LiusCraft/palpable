package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	schemaPath   = "../../../protocol/palpable.schema.json"
	examplesGlob = "../../../protocol/examples/*.json"
)

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	abs, err := filepath.Abs(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	sch, err := c.Compile(abs)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return sch
}

func readJSON(t *testing.T, path string) any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	v, err := jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return v
}

// 例子文件：invalid- 开头必须被 schema 拒绝，其余必须通过。
func TestExamplesMatchSchema(t *testing.T) {
	sch := compileSchema(t)
	files, err := filepath.Glob(examplesGlob)
	if err != nil || len(files) == 0 {
		t.Fatalf("no examples found: %v", err)
	}
	for _, f := range files {
		wantInvalid := strings.HasPrefix(filepath.Base(f), "invalid-")
		err := sch.Validate(readJSON(t, f))
		if wantInvalid && err == nil {
			t.Errorf("%s should be rejected by schema", f)
		}
		if !wantInvalid && err != nil {
			t.Errorf("%s should be accepted: %v", f, err)
		}
	}
}

// 上行例子必须能被 Go 解码；非法例子必须被 Go 拒绝，与 schema 结论一致。
func TestDecodeUpstreamAgreesWithSchema(t *testing.T) {
	files, _ := filepath.Glob(examplesGlob)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var head struct{ Type string }
		_ = json.Unmarshal(raw, &head)

		_, decErr := DecodeUpstream(raw)
		switch {
		case strings.HasPrefix(filepath.Base(f), "invalid-"):
			if decErr == nil {
				t.Errorf("%s: Go decoder accepted an invalid message", f)
			}
		case isUpstream(head.Type):
			if decErr != nil {
				t.Errorf("%s: Go decoder rejected a valid message: %v", f, decErr)
			}
		}
	}
}

// Go 编码出的下行消息必须通过 schema。
func TestEncodedDownstreamMatchesSchema(t *testing.T) {
	sch := compileSchema(t)
	ap := DefaultAudioParams()
	msgs := []Message{
		&HelloAck{Type: TypeHelloAck, SessionID: "s-1", AudioParams: ap},
		&AudioDownlink{Type: TypeAudioDownlink, SampleRate: 24000, FrameDuration: 60},
		&Light{Type: TypeLight, State: "listening", Color: "#7fb8ff", Pattern: "breathe"},
		&Haptic{Type: TypeHaptic, Pattern: "tap"},
		&Sound{Type: TypeSound, Name: "connected"},
		&SpeakerState{Type: TypeSpeakerState, State: "start", Source: "peer", From: "plush-002"},
		&Presence{Type: TypePresence, Event: "join", DeviceID: "plush-002", Name: "小兔", Channel: "park"},
		&Error{Type: TypeError, Code: CodeBadMessage, Message: "x"},
	}
	for _, m := range msgs {
		raw, err := Encode(m)
		if err != nil {
			t.Fatal(err)
		}
		v, err := jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		if err := sch.Validate(v); err != nil {
			t.Errorf("%s encoded message violates schema: %v\n%s", m.MsgType(), err, raw)
		}
	}
}

func TestDecodeUpstreamRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"not json":         `{`,
		"unknown type":     `{"type":"nope"}`,
		"downstream type":  `{"type":"light","state":"idle"}`,
		"unknown field":    `{"type":"wake","source":"touch","x":1}`,
		"bad source":       `{"type":"wake","source":"kick"}`,
		"bad device id":    `{"type":"hello","device_id":"a b","audio_params":{"format":"opus","sample_rate":16000,"channels":1,"frame_duration":60},"capabilities":[]}`,
		"non-mac id":       `{"type":"hello","device_id":"plush-001","audio_params":{"format":"opus","sample_rate":16000,"channels":1,"frame_duration":60},"capabilities":[]}`,
		"bad skin":         `{"type":"hello","device_id":"02:00:00:00:00:01","skin":"Dragon!","audio_params":{"format":"opus","sample_rate":16000,"channels":1,"frame_duration":60},"capabilities":[]}`,
		"bad audio params": `{"type":"hello","device_id":"02:00:00:00:00:01","audio_params":{"format":"opus","sample_rate":8000,"channels":1,"frame_duration":60},"capabilities":[]}`,
		"lat out of range": `{"type":"location","lat":91,"lng":0}`,
		"strength > 1":     `{"type":"proximity","peers":[{"device_id":"b","strength":2}]}`,
	}
	for name, in := range cases {
		if _, err := DecodeUpstream([]byte(in)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func isUpstream(t string) bool {
	switch t {
	case TypeHello, TypeWake, TypeMicState, TypeProximity, TypeLocation:
		return true
	}
	return false
}
