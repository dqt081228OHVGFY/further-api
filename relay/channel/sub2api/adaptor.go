package sub2api

import (
	"github.com/dqt081228OHVGFY/further-api/relay/channel/furtherapi"
)

type Adaptor struct {
	furtherapi.Adaptor
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
