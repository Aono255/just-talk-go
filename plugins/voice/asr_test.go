package voice

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/c/just-talk-go/config"
	"github.com/coder/websocket"
)

func TestASRVersionsUseSelectedHandshakeResourceAndTheSamePCMProtocol(t *testing.T) {
	for _, resource := range config.ASRResources() {
		t.Run(resource.ID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			received := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Api-Resource-Id") != resource.ID || r.Header.Get("X-Api-App-Key") != "test-app" || r.Header.Get("X-Api-Access-Key") != "test-token" {
					t.Error("WebSocket handshake changed the model or legacy credentials")
				}
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.CloseNow()
				_, frame, err := conn.Read(ctx)
				if err != nil {
					t.Error(err)
					return
				}
				received <- frame
			}))
			defer server.Close()
			client := NewASRClient(ASRConfig{AppKey: "test-app", AccessKey: "test-token", ResourceID: resource.ID}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err := client.connect(ctx, strings.Replace(server.URL, "http://", "ws://", 1)); err != nil {
				t.Fatal(err)
			}
			defer client.conn.CloseNow()
			select {
			case frame := <-received:
				if len(frame) < 8 || int(binary.BigEndian.Uint32(frame[4:8])) != len(frame)-8 {
					t.Fatal("invalid initial frame")
				}
				var payload struct {
					Audio struct {
						Format              string
						Rate, Bits, Channel int
					}
					Request struct {
						ModelName string `json:"model_name"`
					}
				}
				if json.Unmarshal(frame[8:], &payload) != nil || payload.Request.ModelName != "bigmodel" || payload.Audio.Format != "pcm" || payload.Audio.Rate != 16000 || payload.Audio.Bits != 16 || payload.Audio.Channel != 1 {
					t.Fatal("model switching changed the audio protocol")
				}
			case <-ctx.Done():
				t.Fatal("ASR initialization did not arrive")
			}
		})
	}
}

func TestASRRequestSelectsCloudTableOrInlineHotwords(t *testing.T) {
	for _, mode := range []string{"empty", "inline", "cloud", "both"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			received := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.CloseNow()
				_, frame, err := conn.Read(ctx)
				if err != nil {
					t.Error(err)
					return
				}
				received <- frame
			}))
			defer server.Close()
			cfg := ASRConfig{}
			if mode == "inline" || mode == "both" {
				cfg.Hotwords = []string{"Codex"}
			}
			if mode == "cloud" || mode == "both" {
				cfg.BoostingTableID = " cloud-table-id "
			}
			client := NewASRClient(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
			var err error
			client.conn, _, err = websocket.Dial(ctx, strings.Replace(server.URL, "http://", "ws://", 1), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer client.conn.CloseNow()
			if err := client.sendFullClientRequest(ctx); err != nil {
				t.Fatal(err)
			}
			var frame []byte
			select {
			case frame = <-received:
			case <-ctx.Done():
				t.Fatal("initial ASR request was not received")
			}
			if len(frame) < 8 || int(binary.BigEndian.Uint32(frame[4:8])) != len(frame)-8 {
				t.Fatal("invalid ASR request framing")
			}
			var payload struct {
				Request struct{ Corpus map[string]json.RawMessage }
			}
			if err := json.Unmarshal(frame[8:], &payload); err != nil {
				t.Fatal(err)
			}
			corpus := payload.Request.Corpus
			switch mode {
			case "cloud", "both":
				if len(corpus) != 1 || string(corpus["boosting_table_id"]) != `"cloud-table-id"` {
					t.Fatal("cloud table ID was omitted or overridden by inline hotwords")
				}
			case "inline":
				var value string
				if json.Unmarshal(corpus["context"], &value) != nil || !strings.Contains(value, "Codex") || len(corpus) != 1 {
					t.Fatal("legacy inline hotwords changed")
				}
			case "empty":
				if len(corpus) != 0 {
					t.Fatal("unset options created an ASR corpus")
				}
			}
		})
	}
}

func TestParseResponseClosesFinalForEmptyText(t *testing.T) {
	client := NewASRClient(ASRConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	client.parseResponse(testASRResponse(t, 0x03, "", false))

	select {
	case <-client.Final():
	default:
		t.Fatal("empty final response did not close Final")
	}
}

func TestParseResponseDoesNotCloseFinalForDefiniteUtterance(t *testing.T) {
	client := NewASRClient(ASRConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	client.parseResponse(testASRResponse(t, 0x01, "partial", true))

	select {
	case <-client.Final():
		t.Fatal("definite utterance closed Final before the last packet")
	default:
	}

	select {
	case result := <-client.Results():
		if !result.IsFinal {
			t.Fatal("definite utterance was not marked final in result stream")
		}
	default:
		t.Fatal("definite utterance result was not published")
	}
}

func TestSendAudioHonorsContextWhileWaitingForWriter(t *testing.T) {
	client := NewASRClient(ASRConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	<-client.writeSlot

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := client.SendAudio(ctx, nil, true)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SendAudio error = %v, want context deadline exceeded", err)
	}
}

func testASRResponse(t *testing.T, flags byte, text string, definite bool) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"result": map[string]any{
			"text": text,
			"utterances": []map[string]any{
				{"text": text, "definite": definite},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	response := make([]byte, 12+len(payload))
	response[0] = hdrVersion | hdrHeaderSize
	response[1] = 0x90 | flags
	binary.BigEndian.PutUint32(response[4:8], ^uint32(0))
	binary.BigEndian.PutUint32(response[8:12], uint32(len(payload)))
	copy(response[12:], payload)
	return response
}
