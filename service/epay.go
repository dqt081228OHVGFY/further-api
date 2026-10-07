package service

import (
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"strings"
)

func GetCallbackAddress() string {
	for _, candidate := range []string{operation_setting.CustomCallbackAddress, system_setting.ServerAddress} {
		if value := strings.TrimSpace(candidate); value != "" {
			return strings.TrimRight(value, "/")
		}
	}
	return ""
}
