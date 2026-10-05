// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package openai

import (
	"testing"
)

func TestParseConfigDefaults(t *testing.T) {
	c, err := ParseConfig(map[string]string{"api_key": "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("base = %s", c.BaseURL)
	}
	if c.Model != "whisper-1" {
		t.Fatalf("model = %s", c.Model)
	}
}

func TestParseConfigRequiresKey(t *testing.T) {
	if _, err := ParseConfig(map[string]string{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseConfigCustomEndpoint(t *testing.T) {
	c, err := ParseConfig(map[string]string{
		"api_key":  "k",
		"base_url": "http://127.0.0.1:8000/v1/",
		"model":    "whisper-small",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != "http://127.0.0.1:8000/v1" {
		t.Fatalf("base = %s", c.BaseURL)
	}
	if c.Model != "whisper-small" {
		t.Fatalf("model = %s", c.Model)
	}
}

func TestAvailable(t *testing.T) {
	e, err := NewFromMap(map[string]string{"api_key": "k"})
	if err != nil {
		t.Fatal(err)
	}
	ok, detail := e.Available(nil)
	if !ok || detail == "" {
		t.Fatalf("ok=%v detail=%q", ok, detail)
	}
	if e.Name() != "openai" {
		t.Fatalf("name = %s", e.Name())
	}
}
