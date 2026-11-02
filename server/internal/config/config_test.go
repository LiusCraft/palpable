package config

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad(t *testing.T) {
	cfg, err := Load(write(t, "addr: \":9000\"\nlinx:\n  ws_url: wss://x/ws\n  token: t\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9000" || !cfg.Linx.Enabled() || cfg.Linx.Token != "t" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" || cfg.Linx.Enabled() || !cfg.Demo.VoiceConsent {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadEmptyFileKeepsDefaults(t *testing.T) {
	cfg, err := Load(write(t, ""))
	if err != nil || cfg.Addr != ":8080" {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	if _, err := Load(write(t, "linx:\n  wsurl: x\n")); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestLoadRejectsReadFailure(t *testing.T) {
	// 目录不是 YAML 文件；ReadFile 会返回 EISDIR 而不是 ErrNotExist，必须上抛。
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("expected error for unreadable path")
	}
}

// 示例配置随字段一起改；拼错或漏改的键会被 KnownFields 拒绝。
func TestExampleConfigLoads(t *testing.T) {
	cfg, err := Load("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WeChat.Enabled() {
		t.Fatal("example config must not ship WeChat credentials")
	}
}

func TestOpsValidate(t *testing.T) {
	long := "0123456789abcdef"
	for name, o := range map[string]Ops{
		"empty is fine":        {},
		"token only":           {Token: long},
		"full":                 {Token: long, AdminURL: "http://admin", AdminToken: long, OwnerHMACKey: long},
		"hook without token":   {AdminURL: "http://admin", OwnerHMACKey: long},
		"hook without hmac":    {AdminURL: "http://admin", AdminToken: long},
		"weak ops token":       {Token: "short"},
		"hook with weak token": {AdminURL: "http://admin", AdminToken: "short", OwnerHMACKey: long},
	} {
		err := o.Validate()
		wantOK := name == "empty is fine" || name == "token only" || name == "full"
		if (err == nil) != wantOK {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	cfg, err := Load(write(t, "ops:\n  token: abc\n"))
	if err != nil || cfg.Ops.Token != "abc" {
		t.Fatalf("ops section not parsed: %+v %v", cfg.Ops, err)
	}
}
