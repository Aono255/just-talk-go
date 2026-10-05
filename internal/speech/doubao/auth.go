// Copyright (C) 2026 conglinyizhi
// SPDX-License-Identifier: GPL-3.0-only
//
// This file is part of Just Talk Plus.

package doubao

import (
	"net/http"

	"github.com/google/uuid"
)

// setAuthHeaders 写入鉴权与资源头；返回 request-id。
func setAuthHeaders(h http.Header, c Config, resourceID, requestID string) string {
	if requestID == "" {
		requestID = uuid.NewString()
	}
	h.Set("X-Api-Request-Id", requestID)
	h.Set("X-Api-Resource-Id", resourceID)
	h.Set("X-Api-Sequence", "-1")
	if c.APIKey != "" {
		h.Set("X-Api-Key", c.APIKey)
	} else {
		// 旧版双头；文档 TTS/ASR 混用 App-Id / App-Key，两边都写一份
		h.Set("X-Api-App-Id", c.AppID)
		h.Set("X-Api-App-Key", c.AppID)
		h.Set("X-Api-Access-Key", c.AccessKey)
	}
	return requestID
}
