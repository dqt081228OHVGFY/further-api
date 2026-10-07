package controller

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/i18n"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/service/business"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

type OptionUpdateRequest struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type passkeyDomainOptionRequest struct {
	RPID                *string `json:"rp_id"`
	LegacyRPIDs         *string `json:"legacy_rp_ids"`
	Origins             *string `json:"origins"`
	Preview             bool    `json:"preview"`
	RemovalConfirmation string  `json:"removal_confirmation"`
}

func GetOptions(c *gin.Context) {
	options, err := service.ReadWorkspaceOptions()
	if err != nil {
		common.SysError("读取网关设置: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "系统设置加载失败，请稍后重试"})
		return
	}
	common.ApiSuccess(c, options)
}

func UpdateOption(c *gin.Context) {
	var request OptionUpdateRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "设置请求格式无效"})
		return
	}
	value, err := business.GatewayOptionValue(request.Value)
	if err == nil {
		err = business.ValidateGatewayOptionCommand(request.Key, value)
	}
	if err != nil {
		writeGatewayOptionError(c, err)
		return
	}
	if model.IsPasskeyDomainOption(request.Key) {
		change, err := model.UpdatePasskeyDomainOptionsContext(c.Request.Context(), map[string]string{request.Key: value}, false, "")
		if err != nil {
			writePasskeyDomainSettingsError(c, err)
			recordPasskeyDomainAudit(c, change, false, err)
			return
		}
		recordPasskeyDomainAudit(c, change, false, nil)
		common.ApiSuccess(c, change)
		return
	}
	if err := model.UpdateOptionContext(c.Request.Context(), request.Key, value); err != nil {
		writeGatewayOptionError(c, err)
		return
	}
	recordManageAudit(c, "option.update", map[string]any{"key": request.Key})
	common.ApiSuccess(c, nil)
}

func writeGatewayOptionError(c *gin.Context, err error) {
	var input *business.GatewayOptionInputError
	if errors.As(err, &input) {
		c.JSON(input.Status, gin.H{"success": false, "code": input.Code, "message": input.Message})
		return
	}
	if errors.Is(err, system_setting.ErrPasskeyRPIDInvalid) {
		writeSecurityOperationError(c, err)
		return
	}
	common.ApiError(c, err)
}

func UpdatePasskeyDomains(c *gin.Context) {
	var request passkeyDomainOptionRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil || request.RPID == nil || request.LegacyRPIDs == nil || request.Origins == nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	change, err := model.UpdatePasskeyDomainOptionsContext(c.Request.Context(), map[string]string{"passkey.rp_id": *request.RPID, "passkey.legacy_rp_ids": *request.LegacyRPIDs, "passkey.origins": *request.Origins}, request.Preview, request.RemovalConfirmation)
	if err != nil {
		writePasskeyDomainSettingsError(c, err)
		if !request.Preview {
			recordPasskeyDomainAudit(c, change, request.RemovalConfirmation != "", err)
		}
		return
	}
	if !request.Preview {
		recordPasskeyDomainAudit(c, change, request.RemovalConfirmation != "", nil)
	}
	common.ApiSuccess(c, change)
}

func writePasskeyDomainSettingsError(c *gin.Context, err error) {
	var removal *model.PasskeyDomainRemovalError
	if errors.As(err, &removal) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "code": "PASSKEY_RP_ID_REMOVAL_CONFIRMATION_REQUIRED", "message": i18n.T(c, i18n.MsgPasskeyRPIDRemovalConfirmation), "data": removal.Change})
		return
	}
	if errors.Is(err, system_setting.ErrPasskeyRPIDInvalid) {
		writeSecurityOperationError(c, err)
		return
	}
	common.ApiError(c, err)
}
