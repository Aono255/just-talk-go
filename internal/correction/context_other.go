//go:build !darwin

package correction

import "errors"

func capturePlatform() (*Target, error) {
	return nil, errors.New("当前 Codex 聊天读取暂只支持 macOS，未执行无上下文纠错")
}
