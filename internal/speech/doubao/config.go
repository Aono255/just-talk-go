// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package doubao

import (
	"fmt"
	"strings"
)

// 配置键（speech.Config.config map）：
//
//	api_key          新版控制台 X-Api-Key（优先）
//	app_id           旧版 X-Api-App-Id / App-Key
//	access_key       旧版 X-Api-Access-Key
//	model_version    1.0 | 2.0（默认 1.0；与旧版 just-talk 的 bigasr 资源一致，2.0 用 seedasr）
//	stream_mode      nostream | async（默认 async；文档仅这两个 WebSocket 端点，无 stream）
//	stream_billing   duration | concurrent（默认 duration）
//	file_mode        flash | standard（默认 flash；标准版需公网 URL，本机上传优先 flash）
//	resource_id      显式覆盖流式 X-Api-Resource-Id（默认由 model_version+billing 推导）
//	language         可选 BCP-47
//	enable_itn       true/false（默认 true）
//	enable_punc      true/false（默认 true；文档默认 false，语音输入工具开启标点更友好）
//	enable_ddc       true/false（默认 false）
//	enable_lid       true/false（默认 false；中英文及方言识别，中英混说建议开启）
//	enable_nonstream true/false（默认 true；二遍识别，仅流式模式 async 生效）
//	boosting_table_name / boosting_table_id       热词词表（控制台自学习平台）
//	correct_table_name / correct_table_id         替换词词表
//	regex_correct_table_name / regex_correct_table_id  正则替换词表
//	base_host        默认 openspeech.bytedance.com（官方固定域名；仅在自建代理/网关时覆盖）

type Config struct {
	APIKey                string
	AppID                 string
	AccessKey             string
	ModelVersion          string // 1.0 | 2.0
	StreamMode            string // nostream | async
	StreamBilling         string // duration | concurrent
	FileMode              string // flash | standard
	ResourceID            string // 显式资源 ID 覆盖
	Language              string
	EnableITN             bool
	EnablePunc            bool
	EnableDDC             bool
	EnableLID             bool
	EnableNonstream       bool
	BoostingTableName     string
	BoostingTableID       string
	CorrectTableName      string
	CorrectTableID        string
	RegexCorrectTableName string
	RegexCorrectTableID   string
	BaseHost              string
}

func ParseConfig(m map[string]string) (Config, error) {
	if m == nil {
		m = map[string]string{}
	}
	c := Config{
		APIKey:                strings.TrimSpace(m["api_key"]),
		AppID:                 firstNonEmpty(m["app_id"], m["app_key"]),
		AccessKey:             strings.TrimSpace(m["access_key"]),
		ModelVersion:          strings.TrimSpace(m["model_version"]),
		StreamMode:            strings.TrimSpace(m["stream_mode"]),
		StreamBilling:         strings.TrimSpace(m["stream_billing"]),
		FileMode:              strings.TrimSpace(m["file_mode"]),
		ResourceID:            strings.TrimSpace(m["resource_id"]),
		Language:              strings.TrimSpace(m["language"]),
		BaseHost:              strings.TrimSpace(m["base_host"]),
		EnableITN:             parseBoolDefault(m["enable_itn"], true),
		EnablePunc:            parseBoolDefault(m["enable_punc"], true),
		EnableDDC:             parseBoolDefault(m["enable_ddc"], false),
		EnableLID:             parseBoolDefault(m["enable_lid"], false),
		EnableNonstream:       parseBoolDefault(m["enable_nonstream"], true),
		BoostingTableName:     strings.TrimSpace(m["boosting_table_name"]),
		BoostingTableID:       strings.TrimSpace(m["boosting_table_id"]),
		CorrectTableName:      strings.TrimSpace(m["correct_table_name"]),
		CorrectTableID:        strings.TrimSpace(m["correct_table_id"]),
		RegexCorrectTableName: strings.TrimSpace(m["regex_correct_table_name"]),
		RegexCorrectTableID:   strings.TrimSpace(m["regex_correct_table_id"]),
	}
	if c.ModelVersion == "" {
		// 新版 X-Api-Key 通常绑定流式 2.0（seedasr）；旧版凭据保持 1.0（bigasr）兼容。
		if c.APIKey != "" {
			c.ModelVersion = "2.0"
		} else {
			c.ModelVersion = "1.0"
		}
	}
	if c.StreamMode == "" {
		c.StreamMode = "async"
	}
	if c.StreamBilling == "" {
		c.StreamBilling = "duration"
	}
	if c.FileMode == "" {
		c.FileMode = "flash"
	}
	if c.BaseHost == "" {
		c.BaseHost = "openspeech.bytedance.com"
	}
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) validate() error {
	if c.APIKey == "" && (c.AppID == "" || c.AccessKey == "") {
		return fmt.Errorf("doubao: 需要 api_key，或 app_id+access_key")
	}
	switch c.ModelVersion {
	case "1.0", "2.0":
	default:
		return fmt.Errorf("doubao: model_version 须为 1.0 或 2.0")
	}
	switch c.StreamMode {
	case "nostream", "async":
	default:
		return fmt.Errorf("doubao: stream_mode 须为 nostream|async")
	}
	switch c.StreamBilling {
	case "duration", "concurrent":
	default:
		return fmt.Errorf("doubao: stream_billing 须为 duration|concurrent")
	}
	switch c.FileMode {
	case "flash", "standard":
	default:
		return fmt.Errorf("doubao: file_mode 须为 flash|standard")
	}
	return nil
}

func (c Config) hasAuth() bool {
	return c.APIKey != "" || (c.AppID != "" && c.AccessKey != "")
}

// StreamResourceID X-Api-Resource-Id for SAUC streaming.
// 显式配置 resource_id 时直接使用（兼容旧版 just-talk 配置）。
func (c Config) StreamResourceID() string {
	if c.ResourceID != "" {
		return c.ResourceID
	}
	prefix := "volc.seedasr.sauc"
	if c.ModelVersion == "1.0" {
		prefix = "volc.bigasr.sauc"
	}
	if c.StreamBilling == "concurrent" {
		return prefix + ".concurrent"
	}
	return prefix + ".duration"
}

// FileResourceID for AUC file APIs（标准版；极速版固定 turbo）。
func (c Config) FileResourceID() string {
	if c.FileMode == "flash" {
		return "volc.bigasr.auc_turbo"
	}
	if c.ModelVersion == "1.0" {
		return "volc.bigasr.auc"
	}
	return "volc.seedasr.auc"
}

func (c Config) streamPath() string {
	switch c.StreamMode {
	case "async":
		return "/api/v3/sauc/bigmodel_async"
	default:
		return "/api/v3/sauc/bigmodel_nostream"
	}
}

func (c Config) streamURL() string {
	return "wss://" + c.BaseHost + c.streamPath()
}

func (c Config) flashURL() string {
	return "https://" + c.BaseHost + "/api/v3/auc/bigmodel/recognize/flash"
}

func (c Config) submitURL() string {
	return "https://" + c.BaseHost + "/api/v3/auc/bigmodel/submit"
}

func (c Config) queryURL() string {
	return "https://" + c.BaseHost + "/api/v3/auc/bigmodel/query"
}

func parseBoolDefault(s string, def bool) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return def
	}
	switch s {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func firstNonEmpty(a, b string) string {
	a = strings.TrimSpace(a)
	if a != "" {
		return a
	}
	return strings.TrimSpace(b)
}
