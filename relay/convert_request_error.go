package relay

import (
	"errors"
	"net/http"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	kitreasoning "github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/reasoning"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func newConvertRequestFailedError(c *gin.Context, info *relaycommon.RelayInfo, err error) *types.GatewayError {
	var loss *types.ConversionLossError
	if errors.As(err, &loss) {
		info.RecordConversionDiagnostics(c, loss.Diagnostics)
		return types.NewErrorWithStatusCode(err, types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if kitreasoning.IsClientError(err) {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
}
