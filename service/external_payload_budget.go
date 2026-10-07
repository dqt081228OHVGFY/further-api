package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/gin-gonic/gin"
)

var ErrExternalPayloadCapacity = errors.New("网关文件转换资源正忙，请稍后重试。")
var ErrRequestFileCapacity = errors.New("本次请求的文件总量超过配置上限。")

type externalPayloadBudget struct {
	mutex     sync.Mutex
	used      int64
	fileLoads int
	polls     int
}

var externalPayloadResources externalPayloadBudget

type externalPayloadLease struct {
	budget   *externalPayloadBudget
	weight   int64
	kind     string
	active   bool
	released bool
	memory   *common.PayloadMemoryReservation
}

func configuredMB(value, fallback int) int64 {
	if value <= 0 {
		value = fallback
	}
	if int64(value) > (1<<62)/(1<<20) {
		return 1 << 62
	}
	return int64(value) << 20
}

func fileMemoryWeight(rawBytes int64) int64 {
	// Account for decoded data, encoded strings and marshal/clone copies. The
	// admission budget is conservative ownership accounting, not a Go RSS cap.
	encoded := (rawBytes + 2) / 3 * 4
	if encoded > (1<<62)/4 || rawBytes > (1<<62)/2 {
		return 1 << 62
	}
	return encoded*4 + rawBytes*2 + 64<<10
}

func (budget *externalPayloadBudget) acquire(ctx context.Context, weight int64, kind string) (*externalPayloadLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	budget.mutex.Lock()
	defer budget.mutex.Unlock()
	if weight <= 0 {
		return nil, ErrExternalPayloadCapacity
	}
	if kind == "file" && budget.fileLoads >= max(1, constant.FileFetchMaxConcurrency) || kind == "poll" && budget.polls >= max(1, constant.TaskPollMaxConcurrency) {
		return nil, ErrExternalPayloadCapacity
	}
	memory, err := common.ReservePayloadMemory(ctx, weight)
	if err != nil {
		return nil, err
	}
	budget.used += weight
	if kind == "file" {
		budget.fileLoads++
	} else {
		budget.polls++
	}
	return &externalPayloadLease{budget: budget, weight: weight, kind: kind, active: true, memory: memory}, nil
}

func (lease *externalPayloadLease) loaded(actualWeight int64) {
	lease.budget.mutex.Lock()
	defer lease.budget.mutex.Unlock()
	if lease.released {
		return
	}
	if actualWeight >= 0 && actualWeight < lease.weight {
		_ = lease.memory.Resize(actualWeight)
		lease.budget.used -= lease.weight - actualWeight
		lease.weight = actualWeight
	}
	if lease.active && lease.kind == "file" {
		lease.budget.fileLoads--
		lease.active = false
	}
}

func (lease *externalPayloadLease) release() {
	if lease == nil {
		return
	}
	lease.budget.mutex.Lock()
	defer lease.budget.mutex.Unlock()
	if lease.released {
		return
	}
	lease.released = true
	lease.memory.Release()
	lease.budget.used -= lease.weight
	if lease.active {
		if lease.kind == "file" {
			lease.budget.fileLoads--
		} else {
			lease.budget.polls--
		}
	}
}

// AcquireTaskPollResources keeps body/JSON capacity until the caller finishes
// parsing and persisting the observation, including failures and cancellation.
func AcquireTaskPollResources(ctx context.Context) (func(), error) {
	limit := TaskPollResponseLimitBytes()
	if limit > (1<<62)/8 {
		return nil, ErrExternalPayloadCapacity
	}
	lease, err := externalPayloadResources.acquire(ctx, limit*8+64<<10, "poll")
	if err != nil {
		return nil, err
	}
	return lease.release, nil
}

func TaskPollResponseLimitBytes() int64 { return configuredMB(constant.TaskPollMaxResponseMB, 64) }

type requestFileBudget struct {
	mutex                 sync.Mutex
	used, reserved, limit int64
}
type requestFileTicket struct {
	budget  *requestFileBudget
	maximum int64
	once    sync.Once
}

var requestFileBudgetInit sync.Mutex

func (budget *requestFileBudget) remaining() int64 {
	budget.mutex.Lock()
	defer budget.mutex.Unlock()
	return budget.limit - budget.used - budget.reserved
}

func fileBudgetForRequest(c *gin.Context) *requestFileBudget {
	if c == nil {
		return &requestFileBudget{limit: configuredMB(constant.FileFetchRequestMaxMB, 128)}
	}
	requestFileBudgetInit.Lock()
	defer requestFileBudgetInit.Unlock()
	if value, exists := c.Get("further_request_file_budget"); exists {
		if budget, ok := value.(*requestFileBudget); ok {
			return budget
		}
	}
	budget := &requestFileBudget{limit: configuredMB(constant.FileFetchRequestMaxMB, 128)}
	c.Set("further_request_file_budget", budget)
	return budget
}

func (budget *requestFileBudget) reserve(maximum int64) (*requestFileTicket, error) {
	budget.mutex.Lock()
	defer budget.mutex.Unlock()
	remaining := budget.limit - budget.used - budget.reserved
	if maximum <= 0 || maximum > remaining {
		return nil, fmt.Errorf("%w 最大 %d MiB", ErrRequestFileCapacity, budget.limit>>20)
	}
	budget.reserved += maximum
	return &requestFileTicket{budget: budget, maximum: maximum}, nil
}

func (ticket *requestFileTicket) finish(readBytes int64) {
	ticket.once.Do(func() {
		ticket.budget.mutex.Lock()
		defer ticket.budget.mutex.Unlock()
		ticket.budget.reserved -= ticket.maximum
		ticket.budget.used += readBytes
	})
}
