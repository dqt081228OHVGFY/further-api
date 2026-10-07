package service

import (
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func ConvertResponse(c *gin.Context, info *relaycommon.RelayInfo, target types.RelayFormat, response any) (*relayconvert.ResponseResult, error) {
	result, err := relayconvert.ConvertResponse(conversionContext(c), info, target, response)
	if result != nil && info != nil {
		info.RecordConversionDiagnostics(c, result.Diagnostics)
	}
	return result, err
}

func ConvertStreamResponse(c *gin.Context, info *relaycommon.RelayInfo, target types.RelayFormat, response any) (*relayconvert.ResponseResult, error) {
	result, err := relayconvert.ConvertStreamResponse(conversionContext(c), info, target, response)
	if result != nil && info != nil {
		info.RecordConversionDiagnostics(c, result.Diagnostics)
	}
	return result, err
}

func ConvertStreamResponseChunk(c *gin.Context, info *relaycommon.RelayInfo, state *relayconvert.ResponseStreamState, response any) ([]relayconvert.ResponseResult, error) {
	results, err := relayconvert.ConvertStreamResponseChunk(conversionContext(c), info, state, response)
	if state != nil && info != nil {
		info.RecordConversionDiagnostics(c, state.Diagnostics())
	}
	return results, err
}

func FinalizeStreamResponse(c *gin.Context, info *relaycommon.RelayInfo, state *relayconvert.ResponseStreamState) ([]relayconvert.ResponseResult, error) {
	results, err := relayconvert.FinalizeStreamResponse(conversionContext(c), info, state)
	if state != nil && info != nil {
		info.RecordConversionDiagnostics(c, state.Diagnostics())
	}
	return results, err
}
