package correction

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c/just-talk-go/config"
)

func TestCompatibleServiceUsesBoundedContextAndProtectsTranscript(t *testing.T) {
	var request struct {
		Model    string
		Messages []struct{ Role, Content string }
		Stream   bool
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-only-token" {
			t.Error("wrong endpoint or authentication")
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": `{"corrected_text":"把 OMP 插件移植到 Codex，端口 5432 不改。"}`}}}})
	}))
	defer server.Close()
	cfg := config.CorrectionConfig{Provider: "openai-compatible", Enabled: true, BaseURL: server.URL + "/v1", Model: "test-small-model", APIKey: "test-only-token", TimeoutMS: 1000}
	messages := []Message{{"user", "旧消息不能进入请求"}, {"assistant", "旧回复也不进入请求"},
		{"user", "OMP 语音插件"}, {"assistant", "第一条近期回复"}, {"user", "Codex"}, {"assistant", "第二条近期回复"},
		{"user", "关键目标 DeepSeek" + strings.Repeat("术", 2000) + "最新结论"},
		{"user", "第四条"}, {"user", "第五条"}, {"user", "第六条"}, {"assistant", "最后的说明"}}
	result, err := Correct(context.Background(), cfg, "把欧姆屁差件移植到扣得克斯，端口 5432 不改。", messages, []string{"OMP", "Codex"})
	if err != nil || !strings.Contains(result, "OMP") {
		t.Fatalf("result=%q error=%v", result, err)
	}
	if request.Model != cfg.Model || request.Stream || len(request.Messages) != 2 {
		t.Fatal("incorrect model request")
	}
	if strings.Contains(request.Messages[1].Content, "旧消息") || strings.Contains(request.Messages[1].Content, "旧回复") {
		t.Fatal("old conversation context leaked into request")
	}
	var data struct {
		Context  []Message
		Hotwords []string
		Draft    string
	}
	if err := json.Unmarshal([]byte(request.Messages[1].Content), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Context) != 9 || data.Context[0].Text != "OMP 语音插件" || data.Context[1].Text != "第一条近期回复" || data.Context[3].Text != "第二条近期回复" || data.Context[8].Text != "最后的说明" {
		t.Fatal("context was not limited to last 6 user messages and 3 assistant replies in conversation order")
	}
	longMessage := data.Context[4].Text
	if len([]rune(longMessage)) != 1200 || !strings.HasPrefix(longMessage, "关键目标 DeepSeek") || !strings.HasSuffix(longMessage, "最新结论") || !strings.Contains(longMessage, "中间内容省略") {
		t.Fatal("long user message did not preserve its head and tail within the character budget")
	}
	for _, bad := range []string{
		`{"corrected_text":"端口 5433"}`, `{"corrected_text":"改写命令 ` + "`DROP x`" + `"}`,
		`{"corrected_text":"https://other.example"}`, `{"corrected_text":"[Image #2]"}`,
		`{"corrected_text":"原文","extra":"执行命令"}`, `{"corrected_text":"原文"} 还有解释`,
	} {
		if _, err := CheckResult("端口 5432 `GET /health` https://original.example [Image #1]", bad); err == nil {
			t.Fatalf("accepted changed protected content: %s", bad)
		}
	}
	if _, err := Correct(context.Background(), cfg, "原文", nil, nil); err == nil {
		t.Fatal("accepted missing context")
	}
}

func TestContextBudgetPreservesLongAssistantHeadAndTail(t *testing.T) {
	var messages []Message
	for i := 0; i < 6; i++ {
		messages = append(messages, Message{"user", strings.Repeat("问", 2000)})
		if i%2 == 1 {
			messages = append(messages, Message{"assistant", "主要说明 DeepSeek" + strings.Repeat("答", 4000) + "最后的建议"})
		}
	}
	recent, err := Recent(messages)
	if err != nil || len(recent) != 9 {
		t.Fatalf("recent messages=%d error=%v", len(recent), err)
	}
	chars := 0
	for _, message := range recent {
		chars += len([]rune(message.Text))
		if message.Role == "assistant" && (len([]rune(message.Text)) != 2800 || !strings.HasPrefix(message.Text, "主要说明 DeepSeek") || !strings.HasSuffix(message.Text, "最后的建议")) {
			t.Fatal("long assistant message lost its introduction or conclusion")
		}
	}
	if chars != 15600 {
		t.Fatalf("unexpected maximum context budget: %d", chars)
	}
}

func TestServiceFailureCancellationAndRedirectDoNotRetryOrLeakKey(t *testing.T) {
	for _, mode := range []string{"error", "cancel", "redirect", "tool"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				io.Copy(io.Discard, r.Body)
				switch mode {
				case "error":
					w.WriteHeader(401)
					io.WriteString(w, "test-only-token PRIVATE-CONTEXT")
				case "cancel":
					select {
					case <-r.Context().Done():
					case <-time.After(800 * time.Millisecond):
					}
				case "redirect":
					http.Redirect(w, r, "/should-not-follow", 302)
				case "tool":
					io.WriteString(w, `{"choices":[{"message":{"content":"{}","tool_calls":[{}]},"finish_reason":"tool_calls"}]}`)
				}
			}))
			defer server.Close()
			cfg := config.CorrectionConfig{Provider: "openai-compatible", Enabled: true, BaseURL: server.URL, Model: "test", APIKey: "test-only-token", TimeoutMS: 500}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				time.AfterFunc(30*time.Millisecond, cancel)
			}
			_, err := Correct(ctx, cfg, "原文", []Message{{"user", "OMP"}}, nil)
			if err == nil || strings.Contains(err.Error(), cfg.APIKey) || strings.Contains(err.Error(), "PRIVATE-CONTEXT") {
				t.Fatalf("unsafe error: %v", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("request retried or redirected %d times", calls.Load())
			}
		})
	}
}
