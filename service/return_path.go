package service

import (
	"strings"

	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
)

func PaymentReturnURL(suffix string) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	return base + suffix
}
