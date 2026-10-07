package controller

import (
	"strings"

	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
)

func paymentReturnPath(suffix string) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	return base + suffix
}
