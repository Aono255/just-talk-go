//go:build !darwin

package correction

import (
	"errors"
	"io"
)

func capturePlatform() (*Target, error) {
	return nil, errors.New("当前平台尚未接入上下文来源，未执行无上下文修正")
}

func focusHelperPlatform(io.Writer) error {
	return errors.New("当前输入应用检测暂只支持 macOS")
}
