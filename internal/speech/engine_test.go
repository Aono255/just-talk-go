// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package speech

import (
	"context"
	"errors"
	"testing"
)

func TestNullEngine(t *testing.T) {
	e := Null{}
	if e.Name() != "null" {
		t.Fatalf("name=%s", e.Name())
	}
	ok, detail := e.Available(context.Background())
	if ok || detail == "" {
		t.Fatalf("null should be unavailable: ok=%v detail=%q", ok, detail)
	}
	if _, err := e.Speak(context.Background(), SpeakRequest{Text: "hi"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Speak: %v", err)
	}
	if _, err := e.Transcribe(context.Background(), TranscribeRequest{Audio: []byte{1}}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Transcribe: %v", err)
	}
	if _, err := e.ListVoices(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("ListVoices: %v", err)
	}
}

func TestOrNull(t *testing.T) {
	if OrNull(nil).Name() != "null" {
		t.Fatal("nil -> null")
	}
	if OrNull(Null{}).Name() != "null" {
		t.Fatal("null stays")
	}
}

func TestBuildRegistry(t *testing.T) {
	e, err := Build("", nil)
	if err != nil || e.Name() != "null" {
		t.Fatalf("empty type: %v %v", e, err)
	}
	e, err = Build("null", nil)
	if err != nil || e.Name() != "null" {
		t.Fatalf("null type: %v %v", e, err)
	}
	if _, err := Build("no-such-engine", nil); err == nil {
		t.Fatal("unknown type should error")
	}

	// 注册假引擎
	Register("fake", func(cfg map[string]string) (Engine, error) {
		return fakeEngine{id: cfg["id"]}, nil
	})
	e, err = Build("fake", map[string]string{"id": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if e.Name() != "fake:x" {
		t.Fatalf("got %s", e.Name())
	}

	cfg := Config{Type: "fake", Enabled: true, Config: map[string]string{"id": "y"}}
	e, err = BuildFromConfig(cfg)
	if err != nil || e.Name() != "fake:y" {
		t.Fatalf("BuildFromConfig: %v %v", e, err)
	}
	e, err = BuildFromConfig(Config{Type: "fake", Enabled: false})
	if err != nil || e.Name() != "null" {
		t.Fatalf("disabled should null: %v %v", e, err)
	}
}

func TestProbe(t *testing.T) {
	st := Probe(context.Background(), nil)
	if st.Engine != "null" || st.Available {
		t.Fatalf("%+v", st)
	}
	st = Probe(context.Background(), Null{})
	if st.Detail == "" {
		t.Fatal("want detail")
	}
}

type fakeEngine struct{ id string }

func (f fakeEngine) Name() string               { return "fake:" + f.id }
func (f fakeEngine) Capabilities() Capabilities { return Capabilities{TTS: true} }
func (f fakeEngine) Available(context.Context) (bool, string) {
	return true, "ok"
}
func (f fakeEngine) Speak(context.Context, SpeakRequest) (*SpeakResult, error) {
	return nil, ErrUnsupported
}
func (f fakeEngine) Transcribe(context.Context, TranscribeRequest) (*TranscribeResult, error) {
	return nil, ErrUnsupported
}
func (f fakeEngine) ListVoices(context.Context) ([]Voice, error) {
	return nil, ErrUnsupported
}
