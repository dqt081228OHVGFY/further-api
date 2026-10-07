package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSRFProtectionRejectsDeprecatedPrivateIPv6AndTranslationTargets(t *testing.T) {
	policy := &SSRFProtection{DomainFilterMode: false, IpFilterMode: false}
	for _, host := range []string{"fec0::1", "::127.0.0.1", "64:ff9b:1::a00:1", "2002:7f00:1::1"} {
		t.Run(host, func(t *testing.T) { require.Error(t, policy.ValidateNetworkTarget(host, 443)) })
	}
	policy.AllowPrivateIp = true
	require.NoError(t, policy.ValidateNetworkTarget("fec0::1", 443), "operator-managed private network policies remain explicit")
}
