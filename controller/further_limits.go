package controller

import (
	"net/http"
	"strconv"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
)

func furtherPositiveID(c *gin.Context, parameter string) (int, bool) {
	id, err := strconv.Atoi(c.Param(parameter))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "资源 ID 无效"})
		return 0, false
	}
	return id, true
}

func FurtherModelAccessGet(c *gin.Context) {
	id, valid := furtherPositiveID(c, "id")
	if !valid {
		return
	}
	access, err := model.GetFurtherModelAccess(id)
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, access)
}

func FurtherModelAccessList(c *gin.Context) {
	entries, err := model.ListFurtherModelAccesses()
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, entries)
}

func FurtherModelAccessPut(c *gin.Context) {
	id, valid := furtherPositiveID(c, "id")
	if !valid {
		return
	}
	var access model.FurtherModelAccess
	if !furtherBusinessJSON(c, &access) {
		return
	}
	access.ModelID = id
	result, err := model.SaveFurtherModelAccess(access)
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, id, "further.model.access", map[string]any{"subscription_only": access.SubscriptionOnly})
	common.ApiSuccess(c, result)
}

func FurtherChannelPoliciesGet(c *gin.Context) {
	policies, err := model.ListFurtherChannelPolicies()
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, policies)
}

func FurtherChannelPolicyGet(c *gin.Context) {
	id, valid := furtherPositiveID(c, "id")
	if !valid {
		return
	}
	policy, err := model.GetFurtherChannelPolicy(id)
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"channel_id": id, "requests_per_minute": policy.RequestsPerMinute,
		"concurrent_requests": policy.ConcurrentRequests, "quota_total": policy.QuotaTotal,
		"quota_remaining": policy.QuotaRemaining, "quota_reserved": policy.QuotaReserved, "enforced": false})
}

// Resource budgets and independent RPM/concurrency require the complete
// upstream-attempt lifecycle hook. Key weight is enforced by the actual
// multi-key selection path and may be configured independently.
func furtherChannelLimitsSupported(c *gin.Context, values model.FurtherLimitValues, weight int) bool {
	if values.RequestsPerMinute != 0 || values.ConcurrentRequests != 0 || values.QuotaTotal != -1 || values.QuotaRemaining != -1 {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "渠道/上游 Key 的独立额度、RPM、并发执行链尚未启用；上游 Key 权重已生效，套餐限流不受影响"})
		return false
	}
	return true
}

func FurtherChannelPolicyPut(c *gin.Context) {
	id, valid := furtherPositiveID(c, "id")
	if !valid {
		return
	}
	policy := model.FurtherChannelPolicy{ChannelID: id, FurtherLimitValues: model.FurtherLimitValues{QuotaTotal: -1, QuotaRemaining: -1}}
	if !furtherBusinessJSON(c, &policy) {
		return
	}
	policy.ChannelID = id
	if !furtherChannelLimitsSupported(c, policy.FurtherLimitValues, 1) {
		return
	}
	if err := model.SaveFurtherChannelPolicy(policy); err != nil {
		furtherBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, id, "further.channel.policy", map[string]any{"requests_per_minute": policy.RequestsPerMinute, "concurrent_requests": policy.ConcurrentRequests})
	common.ApiSuccess(c, policy)
}

func FurtherChannelKeysGet(c *gin.Context) {
	id, valid := furtherPositiveID(c, "id")
	if !valid {
		return
	}
	keys, err := model.GetFurtherChannelKeys(id)
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, keys)
}

func FurtherChannelKeySave(c *gin.Context) {
	id, valid := furtherPositiveID(c, "id")
	if !valid {
		return
	}
	keyID := 0
	input := model.FurtherChannelKeyInput{Weight: 1, Status: 1, FurtherLimitValues: model.FurtherLimitValues{QuotaTotal: -1, QuotaRemaining: -1}}
	if c.Param("key_id") != "" {
		keyID, valid = furtherPositiveID(c, "key_id")
		if !valid {
			return
		}
		keys, err := model.GetFurtherChannelKeys(id)
		if err != nil {
			furtherBusinessError(c, err)
			return
		}
		for _, key := range keys {
			if key.ID == keyID {
				input.Label, input.Weight, input.Status, input.FurtherLimitValues = key.Label, key.Weight, key.Status, key.FurtherLimitValues
				break
			}
		}
	}
	if !furtherBusinessJSON(c, &input) {
		return
	}
	if !furtherChannelLimitsSupported(c, input.FurtherLimitValues, input.Weight) {
		return
	}
	key, err := model.SaveFurtherChannelKey(id, keyID, input, false)
	if err != nil {
		furtherBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, id, "further.channel.key_update", map[string]any{"key_id": key.ID, "status": key.Status})
	common.ApiSuccess(c, key)
}

func FurtherChannelKeyDelete(c *gin.Context) {
	id, valid := furtherPositiveID(c, "id")
	if !valid {
		return
	}
	keyID, valid := furtherPositiveID(c, "key_id")
	if !valid {
		return
	}
	if _, err := model.SaveFurtherChannelKey(id, keyID, model.FurtherChannelKeyInput{}, true); err != nil {
		furtherBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, id, "further.channel.key_delete", map[string]any{"key_id": keyID})
	common.ApiSuccess(c, gin.H{"deleted": true})
}
