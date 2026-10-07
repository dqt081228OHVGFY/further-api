package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/i18n"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/service/authz"

	"github.com/gin-gonic/gin"
)

// loginMethodFromContext 根据请求路径推导登录方式，用于登录审计日志。
func loginMethodFromContext(c *gin.Context) string {
	if method := c.GetString("login_method"); method != "" {
		return method
	}
	switch c.FullPath() {
	case "/api/user/login":
		return "password"
	case "/api/user/login/2fa":
		return "2fa"
	case "/api/user/passkey/login/finish":
		return "passkey"
	case "/api/oauth/wechat":
		return "wechat"
	case "/api/oauth/telegram/login":
		return "telegram"
	case "/api/oauth/:provider":
		if provider := c.Param("provider"); provider != "" {
			return "oauth:" + provider
		}
		return "oauth"
	default:
		return "unknown"
	}
}

// recordLoginAudit 记录登录成功审计日志（对所有用户启用，仅记录成功，不记录失败）。
func recordLoginAudit(user *model.User, c *gin.Context) {
	method := loginMethodFromContext(c)
	ip := c.ClientIP()
	extra := model.AuditOther{
		LoginMethod: method,
		UserAgent:   c.Request.UserAgent(),
	}
	content := fmt.Sprintf("Logged in successfully via %s", method)
	params := map[string]any{
		"method": method,
	}
	if verifiedMethod := c.GetString("login_verification_method"); verifiedMethod != "" {
		params["verification_method"] = verifiedMethod
	}
	model.RecordLoginLog(user.Id, user.Role, user.Username, content, ip, "login", params, extra, c)
}

// setupLogin evaluates the shared login policy after primary authentication.
// Only a completed Passkey ceremony may go directly to session issuance. A
// pending legacy GitHub binding rewrite travels inside the challenge and is
// written only when the verification completes.
func setupLogin(user *model.User, migration *service.LegacyGitHubMigration, c *gin.Context) {
	challenge, err := service.StartLoginVerification(user, loginMethodFromContext(c), migration)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	if challenge != nil {
		setAuthNoStore(c)
		common.ApiSuccess(c, challenge)
		return
	}
	setupLoginAtAuthVersion(user, user.AuthVersion, c)
}

func setupLoginAtAuthVersion(user *model.User, expectedAuthVersion int64, c *gin.Context) {
	if user == nil || user.Id <= 0 || user.Status != common.UserStatusEnabled {
		common.ApiErrorI18n(c, i18n.MsgAuthUserBanned)
		return
	}
	currentUser, err := model.GetSelfUserById(user.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var bundle *service.AuthBundle
	if expectedAuthVersion > 0 {
		bundle, err = service.CreateLoginSessionAtAuthVersionContext(
			c.Request.Context(),
			user.Id,
			expectedAuthVersion,
			loginMethodFromContext(c),
			c.ClientIP(),
			c.Request.UserAgent(),
		)
	} else {
		bundle, err = service.CreateLoginSessionContext(
			c.Request.Context(),
			user.Id,
			loginMethodFromContext(c),
			c.ClientIP(),
			c.Request.UserAgent(),
		)
	}
	if err != nil {
		writeAuthSessionError(c, err)
		return
	}
	writeLoginResponse(c, currentUser, bundle)
}

func writeLoginResponse(c *gin.Context, user *model.User, bundle *service.AuthBundle) {
	c.Set("login_method", bundle.Session.LoginMethod)
	model.UpdateUserLastLoginAt(user.Id)
	service.WriteRefreshCookie(c, bundle.RefreshToken)
	setAuthNoStore(c)
	recordLoginAudit(user, c)
	c.JSON(http.StatusOK, gin.H{
		"message": "",
		"success": true,
		"data": gin.H{
			"access_token":      bundle.AccessToken,
			"token_type":        bundle.TokenType,
			"access_expires_at": bundle.AccessExpiresAt,
			"session":           bundle.Session,
			"user":              buildSelfUserData(user),
		},
	})
}

func GetAllUsers(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	sortOptions := model.NewUserSortOptions(c.Query("sort_by"), c.Query("sort_order"))
	users, total, err := model.GetAllUsers(pageInfo, sortOptions)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(users)

	common.ApiSuccess(c, pageInfo)
	return
}

func SearchUsers(c *gin.Context) {
	keyword := c.Query("keyword")
	group := c.Query("group")
	var role *int
	if roleStr := c.Query("role"); roleStr != "" {
		if parsed, err := strconv.Atoi(roleStr); err == nil {
			role = &parsed
		}
	}
	var status *int
	if statusStr := c.Query("status"); statusStr != "" {
		if parsed, err := strconv.Atoi(statusStr); err == nil {
			status = &parsed
		}
	}
	pageInfo := common.GetPageQuery(c)
	sortOptions := model.NewUserSortOptions(c.Query("sort_by"), c.Query("sort_order"))
	var users []*model.User
	var total int64
	var err error
	if inactiveDays := c.Query("inactive_days"); inactiveDays != "" {
		days, parseErr := strconv.Atoi(inactiveDays)
		if parseErr != nil || days != 14 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "沉睡用户筛选仅支持近14天无调用"})
			return
		}
		users, total, err = model.SearchInactiveUsers(keyword, group, role, status, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), time.Now().Add(-time.Duration(days)*24*time.Hour).Unix(), sortOptions)
	} else {
		users, total, err = model.SearchUsers(keyword, group, role, status, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), sortOptions)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(users)
	common.ApiSuccess(c, pageInfo)
	return
}

func canManageTargetRole(myRole int, targetRole int) bool {
	return myRole == common.RoleRootUser || myRole > targetRole
}

func GetUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	myRole := c.GetInt("role")
	if !canManageTargetRole(myRole, user.Role) {
		common.ApiErrorI18n(c, i18n.MsgUserNoPermissionSameLevel)
		return
	}
	user.AdminPermissions = authz.Capabilities(user.Id, user.Role)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    user,
	})
	return
}

type TransferAffQuotaRequest struct {
	Quota int `json:"quota" binding:"required"`
}

func TransferAffQuota(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	id := c.GetInt("id")
	user, err := model.GetUserById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tran := TransferAffQuotaRequest{}
	if err := c.ShouldBindJSON(&tran); err != nil {
		common.ApiError(c, err)
		return
	}
	err = user.TransferAffQuotaToQuota(tran.Quota)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgUserTransferFailed, map[string]any{"Error": err.Error()})
		return
	}
	common.ApiSuccessI18n(c, i18n.MsgUserTransferSuccess, nil)
}

func GetAffCode(c *gin.Context) {
	id := c.GetInt("id")
	user, err := model.GetUserById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user.AffCode == "" {
		user.AffCode = common.GetRandomString(4)
		if err := user.Update(false); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    user.AffCode,
	})
	return
}

func GetSelf(c *gin.Context) {
	user, err := model.GetSelfUserByIdContext(c.Request.Context(), c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	data := service.WorkspaceAccountFromUser(user, c.GetInt("role"))
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": data})
}

// Login, refresh and profile reads share the same explicit account DTO.
func buildSelfUserData(user *model.User) service.WorkspaceAccount {
	return service.WorkspaceAccountFromUser(user, user.Role)
}

// 根据用户角色生成默认的边栏配置
func GetUserModels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		id = c.GetInt("id")
	}
	user, err := model.GetUserCache(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	groups := service.GetUserUsableGroups(user.Group)
	group := c.Query("group")
	var groupsToQuery []string
	switch {
	case group == "":
		for g := range groups {
			groupsToQuery = append(groupsToQuery, g)
		}
	case group == "auto":
		if _, ok := groups[group]; ok {
			groupsToQuery = service.GetUserAutoGroup(user.Group)
		}
	default:
		if _, ok := groups[group]; ok {
			groupsToQuery = []string{group}
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    service.GetGroupsEnabledModels(groupsToQuery),
	})
}

func AdminClearUserBinding(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	bindingType := strings.ToLower(strings.TrimSpace(c.Param("binding_type")))
	if bindingType == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	myRole := c.GetInt("role")
	if !canManageTargetRole(myRole, user.Role) {
		common.ApiErrorI18n(c, i18n.MsgUserNoPermissionSameLevel)
		return
	}

	if _, err := model.MutateManagedAccountSecurityContext(c.Request.Context(), managedSecurityOperator(c), model.ManagedSecurityMutation{TargetID: user.Id, Action: "binding-clear", BindingType: bindingType}); err != nil {
		common.ApiError(c, err)
		return
	}

	recordManageAuditFor(c, user.Id, "user.binding_clear", map[string]any{
		"bindingType": bindingType,
		"username":    user.Username,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "success",
	})
}

func DeleteSelf(c *gin.Context) {
	setAuthNoStore(c)
	succeeded := false
	defer func() {
		recordUserSecurityAudit(c, c.GetInt("id"), "user.account_delete", map[string]any{"success": succeeded})
	}()
	if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeAccountDelete}) == nil {
		return
	}
	identity, _ := middleware.GetSessionAuthIdentity(c)
	if err := model.DeleteUserForSessionContext(c.Request.Context(), identity); err != nil {
		if errors.Is(err, model.ErrCannotDeleteRootUser) {
			common.ApiErrorI18n(c, i18n.MsgUserCannotDeleteRootUser)
			return
		}
		writeSecurityOperationError(c, err)
		return
	}
	succeeded = true
	service.ClearRefreshCookie(c)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    gin.H{},
	})
}

var topUpLocks sync.Map
var topUpCreateLock sync.Mutex

type topUpTryLock struct {
	ch chan struct{}
}

func newTopUpTryLock() *topUpTryLock {
	return &topUpTryLock{ch: make(chan struct{}, 1)}
}

func (l *topUpTryLock) TryLock() bool {
	select {
	case l.ch <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l *topUpTryLock) Unlock() {
	select {
	case <-l.ch:
	default:
	}
}

func getTopUpLock(userID int) *topUpTryLock {
	if v, ok := topUpLocks.Load(userID); ok {
		return v.(*topUpTryLock)
	}
	topUpCreateLock.Lock()
	defer topUpCreateLock.Unlock()
	if v, ok := topUpLocks.Load(userID); ok {
		return v.(*topUpTryLock)
	}
	l := newTopUpTryLock()
	topUpLocks.Store(userID, l)
	return l
}
