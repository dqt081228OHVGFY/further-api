package middleware

import (
	"log"
	"os"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/gin-gonic/gin"
)

func ConfigureTrustedProxies(engine *gin.Engine) error {
	trustedProxies, usedDefaults, err := common.ResolveTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		return err
	}
	if usedDefaults {
		log.Print("TRUSTED_PROXIES is unset or blank; forwarded client IP headers are ignored. Configure the exact reverse proxy peer IP/CIDR when using a proxy.")
	}
	return common.ConfigureTrustedProxies(engine, trustedProxies)
}
