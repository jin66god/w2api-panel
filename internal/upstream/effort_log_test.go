package upstream

import (
	"strings"
	"testing"
)

// issue #43：日志要能区分「客户端要 max」与「模型实际只给 high」。
func TestResolveEffortDowngrade(t *testing.T) {
	efforts := map[string][]string{"deepseek-v4.1-flash": {"low", "high"}}
	if got, changed := ResolveEffort("deepseek-v4.1-flash", "max", efforts); got != "high" || !changed {
		t.Fatalf("max -> got %q changed=%v want high/true", got, changed)
	}
	if got, changed := ResolveEffort("deepseek-v4.1-flash", "high", efforts); got != "high" || changed {
		t.Fatalf("already supported: got %q changed=%v want high/false", got, changed)
	}
	// 支持档全部高于请求档：取最低档（与 normalizeReasoningEffort 同口径）。
	if got, _ := ResolveEffort("m", "low", map[string][]string{"m": {"high", "max"}}); got != "high" {
		t.Fatalf("floor case got %q want high", got)
	}
	// 未知模型 / 未知档位 / 空能力：原样返回，不改写。
	if got, changed := ResolveEffort("unknown", "max", efforts); got != "max" || changed {
		t.Fatalf("unknown model got %q changed=%v", got, changed)
	}
	if got, changed := ResolveEffort("deepseek-v4.1-flash", "turbo", efforts); got != "turbo" || changed {
		t.Fatalf("unknown effort got %q changed=%v", got, changed)
	}
	if got, changed := ResolveEffort("m", "max", nil); got != "max" || changed {
		t.Fatalf("nil efforts got %q changed=%v", got, changed)
	}
}

// RequestHintsOf 一次解析同时给出工具白名单与客户端原始档位。
func TestRequestHintsOf(t *testing.T) {
	body := []byte(`{"model":"m","reasoning_effort":"max","tools":[{"type":"function","function":{"name":"Read"}}],"messages":[{"role":"user","content":"hi"}]}`)
	h := RequestHintsOf(body)
	if !h.ToolNames["Read"] {
		t.Fatalf("tool name lost: %+v", h.ToolNames)
	}
	if h.RequestedEffort != "max" {
		t.Fatalf("requested effort=%q want max", h.RequestedEffort)
	}
	// camelCase 变体。
	h = RequestHintsOf([]byte(`{"model":"m","reasoningEffort":"low","messages":[]}`))
	if h.RequestedEffort != "low" {
		t.Fatalf("camel effort=%q want low", h.RequestedEffort)
	}
	// 未携带档位时为空（日志据此不显示，避免伪装成某个档位）。
	h = RequestHintsOf([]byte(`{"model":"m","messages":[]}`))
	if h.RequestedEffort != "" || h.ToolNames != nil {
		t.Fatalf("empty request hints=%+v", h)
	}
	// 解析失败不编造。
	if h = RequestHintsOf([]byte("{broken")); h.RequestedEffort != "" || h.ToolNames != nil {
		t.Fatalf("broken body hints=%+v", h)
	}
}

// 端到端：出站 body 的档位就是 ResolveEffortFor 报告的值（同一口径的自证）。
func TestResolveEffortMatchesPrepareBody(t *testing.T) {
	efforts := map[string][]string{"deepseek-v4.1-flash": {"low", "high"}}
	in := []byte(`{"model":"deepseek-v4.1-flash","reasoning_effort":"max","messages":[{"role":"user","content":"hi"}]}`)
	eff, _ := ResolveEffort("deepseek-v4.1-flash", "max", efforts)
	if eff != "high" {
		t.Fatalf("resolve=%q", eff)
	}
	out := string(PrepareBodyOptWithEfforts(in, false, efforts))
	if !strings.Contains(out, `"reasoning_effort":"high"`) {
		t.Fatalf("出站 body 档位与日志口径不一致: %s", out)
	}
}
