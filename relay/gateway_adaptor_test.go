package relay

import (
	"testing"

	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/furtherapi"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/sub2api"
	"github.com/stretchr/testify/assert"
)

func TestGatewayAdaptorMigrationPreservesAPITypeContract(t *testing.T) {
	adaptor := GetAdaptor(constant.APITypeGateway)
	assert.IsType(t, &furtherapi.Adaptor{}, adaptor)
	assert.Equal(t, "further-api", adaptor.GetChannelName())
	assert.IsType(t, &sub2api.Adaptor{}, GetAdaptor(constant.APITypeSub2API))
}
