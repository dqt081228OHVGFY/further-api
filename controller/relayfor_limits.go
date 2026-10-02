package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func relayForPositiveID(c *gin.Context, parameter string) (int, bool) {
	id, err := strconv.Atoi(c.Param(parameter))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "资源 ID 无效"})
		return 0, false
	}
	return id, true
}

func RelayForModelAccessGet(c *gin.Context) {
	id, valid := relayForPositiveID(c, "id")
	if !valid {
		return
	}
	access, err := model.GetRelayForModelAccess(id)
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, access)
}

func RelayForModelAccessList(c *gin.Context) {
	entries, err := model.ListRelayForModelAccesses()
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, entries)
}

func RelayForModelAccessPut(c *gin.Context) {
	id, valid := relayForPositiveID(c, "id")
	if !valid {
		return
	}
	var access model.RelayForModelAccess
	if !relayForBusinessJSON(c, &access) {
		return
	}
	access.ModelID = id
	result, err := model.SaveRelayForModelAccess(access)
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, id, "relayfor.model.access", map[string]any{"subscription_only": access.SubscriptionOnly})
	common.ApiSuccess(c, result)
}

func RelayForChannelPoliciesGet(c *gin.Context) {
	policies, err := model.ListRelayForChannelPolicies()
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, policies)
}

func RelayForChannelPolicyGet(c *gin.Context) {
	id, valid := relayForPositiveID(c, "id")
	if !valid {
		return
	}
	policy, err := model.GetRelayForChannelPolicy(id)
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"channel_id": id, "requests_per_minute": policy.RequestsPerMinute,
		"concurrent_requests": policy.ConcurrentRequests, "quota_total": policy.QuotaTotal,
		"quota_remaining": policy.QuotaRemaining, "quota_reserved": policy.QuotaReserved, "enforced": false})
}

// Resource budgets and independent RPM/concurrency require the complete
// upstream-attempt lifecycle hook. Key weight is enforced by the actual
// multi-key selection path and may be configured independently.
func relayForChannelLimitsSupported(c *gin.Context, values model.RelayForLimitValues, weight int) bool {
	if values.RequestsPerMinute != 0 || values.ConcurrentRequests != 0 || values.QuotaTotal != -1 || values.QuotaRemaining != -1 {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "渠道/上游 Key 的独立额度、RPM、并发执行链尚未启用；上游 Key 权重已生效，套餐限流不受影响"})
		return false
	}
	return true
}

func RelayForChannelPolicyPut(c *gin.Context) {
	id, valid := relayForPositiveID(c, "id")
	if !valid {
		return
	}
	policy := model.RelayForChannelPolicy{ChannelID: id, RelayForLimitValues: model.RelayForLimitValues{QuotaTotal: -1, QuotaRemaining: -1}}
	if !relayForBusinessJSON(c, &policy) {
		return
	}
	policy.ChannelID = id
	if !relayForChannelLimitsSupported(c, policy.RelayForLimitValues, 1) {
		return
	}
	if err := model.SaveRelayForChannelPolicy(policy); err != nil {
		relayForBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, id, "relayfor.channel.policy", map[string]any{"requests_per_minute": policy.RequestsPerMinute, "concurrent_requests": policy.ConcurrentRequests})
	common.ApiSuccess(c, policy)
}

func RelayForChannelKeysGet(c *gin.Context) {
	id, valid := relayForPositiveID(c, "id")
	if !valid {
		return
	}
	keys, err := model.GetRelayForChannelKeys(id)
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	common.ApiSuccess(c, keys)
}

func RelayForChannelKeySave(c *gin.Context) {
	id, valid := relayForPositiveID(c, "id")
	if !valid {
		return
	}
	keyID := 0
	input := model.RelayForChannelKeyInput{Weight: 1, Status: 1, RelayForLimitValues: model.RelayForLimitValues{QuotaTotal: -1, QuotaRemaining: -1}}
	if c.Param("key_id") != "" {
		keyID, valid = relayForPositiveID(c, "key_id")
		if !valid {
			return
		}
		keys, err := model.GetRelayForChannelKeys(id)
		if err != nil {
			relayForBusinessError(c, err)
			return
		}
		for _, key := range keys {
			if key.ID == keyID {
				input.Label, input.Weight, input.Status, input.RelayForLimitValues = key.Label, key.Weight, key.Status, key.RelayForLimitValues
				break
			}
		}
	}
	if !relayForBusinessJSON(c, &input) {
		return
	}
	if !relayForChannelLimitsSupported(c, input.RelayForLimitValues, input.Weight) {
		return
	}
	key, err := model.SaveRelayForChannelKey(id, keyID, input, false)
	if err != nil {
		relayForBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, id, "relayfor.channel.key_update", map[string]any{"key_id": key.ID, "status": key.Status})
	common.ApiSuccess(c, key)
}

func RelayForChannelKeyDelete(c *gin.Context) {
	id, valid := relayForPositiveID(c, "id")
	if !valid {
		return
	}
	keyID, valid := relayForPositiveID(c, "key_id")
	if !valid {
		return
	}
	if _, err := model.SaveRelayForChannelKey(id, keyID, model.RelayForChannelKeyInput{}, true); err != nil {
		relayForBusinessError(c, err)
		return
	}
	recordManageAuditFor(c, id, "relayfor.channel.key_delete", map[string]any{"key_id": keyID})
	common.ApiSuccess(c, gin.H{"deleted": true})
}
