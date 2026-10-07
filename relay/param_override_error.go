package relay

import (
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
)

func gatewayErrorFromParamOverride(err error) *types.GatewayError {
	if fixedErr, ok := relaycommon.AsParamOverrideReturnError(err); ok {
		return relaycommon.GatewayErrorFromParamOverride(fixedErr)
	}
	return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
}
