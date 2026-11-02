package service

import (
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/operation_setting"
	"github.com/TranNgocPhuongHieu/LLM-gateway/setting/system_setting"
)

func GetCallbackAddress() string {
	if operation_setting.CustomCallbackAddress == "" {
		return system_setting.ServerAddress
	}
	return operation_setting.CustomCallbackAddress
}
