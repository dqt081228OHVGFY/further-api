package controller

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
)

type channelBalanceCycleSummary struct {
	Checked   int `json:"checked"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

func runChannelBalanceCycle(ctx context.Context) (channelBalanceCycleSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return channelBalanceCycleSummary{}, err
	}
	if model.DB == nil {
		return channelBalanceCycleSummary{}, errors.New("渠道余额数据服务不可用。")
	}
	workers := 4
	if common.RequestInterval > 0 {
		workers = 1
	}
	jobs := make(chan *model.Channel)
	var checked, succeeded, failed atomic.Int64
	var pending sync.WaitGroup
	for range workers {
		pending.Add(1)
		go func() {
			defer pending.Done()
			for candidate := range jobs {
				if ctx.Err() != nil {
					return
				}
				checked.Add(1)
				result, err := updateChannelBalanceContext(ctx, candidate)
				if err != nil {
					failed.Add(1)
				} else {
					succeeded.Add(1)
					if result.RawResponse == "" && result.Balance <= 0 {
						service.DisableChannel(*types.NewChannelError(candidate.Id, candidate.Type, candidate.Name,
							candidate.ChannelInfo.IsMultiKey, "", candidate.GetAutoBan()), "余额不足")
					}
				}
				if !waitChannelBalanceInterval(ctx, common.RequestInterval) {
					return
				}
			}
		}()
	}
	load := func() error {
		const batchSize = 128
		cursor := 0
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			var batch []*model.Channel
			if err := model.DB.WithContext(ctx).Where("id > ? AND status = ?", cursor, common.ChannelStatusEnabled).
				Order("id ASC").Limit(batchSize).Find(&batch).Error; err != nil {
				return err
			}
			if len(batch) == 0 {
				return nil
			}
			cursor = batch[len(batch)-1].Id
			for _, candidate := range batch {
				if candidate == nil || candidate.ChannelInfo.IsMultiKey || candidate.Type == constant.ChannelTypeTaskPlugin {
					continue
				}
				if candidate.Type != constant.ChannelTypeAdvancedCustom && gatewayBalanceReaders[candidate.Type] == nil {
					continue
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case jobs <- candidate:
				}
			}
			if len(batch) < batchSize {
				return nil
			}
		}
	}
	err := load()
	close(jobs)
	pending.Wait()
	if err == nil {
		err = ctx.Err()
	}
	return channelBalanceCycleSummary{Checked: int(checked.Load()), Succeeded: int(succeeded.Load()), Failed: int(failed.Load())}, err
}
