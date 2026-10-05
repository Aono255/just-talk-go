// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package speech

import (
	"fmt"
	"sync"
)

// Factory 按配置构造引擎。cfg 为渠道式键值（API key、endpoint、model 等）。
type Factory func(cfg map[string]string) (Engine, error)

var (
	regMu     sync.RWMutex
	factories = map[string]Factory{}
)

func init() {
	// 内置 null：显式 type=null 或未配置时使用
	Register("null", func(map[string]string) (Engine, error) { return Null{}, nil })
}

// Register 注册引擎类型。重复注册会覆盖（便于测试替换）。
func Register(typ string, f Factory) {
	if typ == "" || f == nil {
		return
	}
	regMu.Lock()
	factories[typ] = f
	regMu.Unlock()
}

// Types 已注册类型名（无序）。
func Types() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(factories))
	for k := range factories {
		out = append(out, k)
	}
	return out
}

// Build 按类型构造引擎。typ 空或 "null" → Null。
func Build(typ string, cfg map[string]string) (Engine, error) {
	if typ == "" || typ == "null" {
		return Null{}, nil
	}
	regMu.RLock()
	f, ok := factories[typ]
	regMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("speech: unknown engine type %q", typ)
	}
	if cfg == nil {
		cfg = map[string]string{}
	}
	return f(cfg)
}

// BuildFromConfig 由 Config 构造引擎；未启用或空类型 → Null。
func BuildFromConfig(c Config) (Engine, error) {
	if !c.Enabled || c.Type == "" || c.Type == "null" {
		return Null{}, nil
	}
	return Build(c.Type, c.Config)
}

// Config 引擎选择的持久化形态。
type Config struct {
	// Type 注册表中的引擎类型；空 = null
	Type    string            `json:"type"`
	Enabled bool              `json:"enabled"`
	Config  map[string]string `json:"config,omitempty"`
}
