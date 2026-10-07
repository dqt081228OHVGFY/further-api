package service

import (
	"errors"
	"math"
	"sort"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/shopspring/decimal"
)

type ReservationQuoteInput struct {
	Base       float64
	GroupRatio float64
	Factors    map[string]float64
	Round      bool
}

type ReservationQuote struct {
	BeforeGroup float64
	Quota       int
}

func PriceReservation(input ReservationQuoteInput) (ReservationQuote, error) {
	for _, value := range []float64{input.Base, input.GroupRatio} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			_, err := common.QuotaFromFloatStrict(value)
			return ReservationQuote{}, err
		}
		if value < 0 {
			return ReservationQuote{}, errors.New("模型单价或分组倍率不能为负数。")
		}
	}
	before := decimal.NewFromFloat(input.Base)
	amount := before.Mul(decimal.NewFromFloat(input.GroupRatio))
	keys := make([]string, 0, len(input.Factors))
	for name := range input.Factors {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		factor := input.Factors[name]
		if factor <= 0 || math.IsNaN(factor) || math.IsInf(factor, 0) {
			continue
		}
		amount = amount.Mul(decimal.NewFromFloat(factor))
	}
	if !input.Round {
		amount = amount.Truncate(0)
	}
	quota, err := common.QuotaFromDecimalStrict(amount)
	var clamp *common.QuotaClamp
	if errors.As(err, &clamp) {
		copy := *clamp
		copy.Op = "QuotaFromFloat"
		if input.Round {
			copy.Op = "QuotaRound"
		}
		err = &copy
	}
	return ReservationQuote{BeforeGroup: input.Base, Quota: quota}, err
}

type RatedUsageComponent struct {
	Units int
	Rate  float64
}

// A charge is a sum of independently priced usage categories followed by
// request-level factors. Numeric validation precedes decimal construction.
func PriceUsageComponents(components []RatedUsageComponent, multiplier float64, requestFactors map[string]float64, extra decimal.Decimal, minimum bool) (int, *common.QuotaClamp) {
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return common.QuotaFromFloatChecked(multiplier)
	}
	total := decimal.Zero
	for _, component := range components {
		if math.IsNaN(component.Rate) || math.IsInf(component.Rate, 0) {
			return common.QuotaFromFloatChecked(component.Rate)
		}
		if component.Units <= 0 {
			continue
		}
		total = total.Add(decimal.NewFromInt(int64(component.Units)).Mul(decimal.NewFromFloat(component.Rate)))
	}
	total = total.Mul(decimal.NewFromFloat(multiplier))
	keys := make([]string, 0, len(requestFactors))
	for name := range requestFactors {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		factor := requestFactors[name]
		if factor > 0 && !math.IsNaN(factor) && !math.IsInf(factor, 0) {
			total = total.Mul(decimal.NewFromFloat(factor))
		}
	}
	total = total.Add(extra)
	if minimum && multiplier != 0 && total.LessThanOrEqual(decimal.Zero) {
		total = decimal.NewFromInt(1)
	}
	return common.QuotaFromDecimalChecked(total)
}
