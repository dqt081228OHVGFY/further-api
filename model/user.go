package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

const UserNameMaxLength = 20

var userSortColumns = map[string]string{
	"id":            "id",
	"username":      "username",
	"quota":         "quota",
	"group":         "group",
	"created_at":    "created_at",
	"last_login_at": "last_login_at",
}

type UserSortOptions struct {
	SortBy    string
	SortOrder string
}

func NewUserSortOptions(sortBy string, sortOrder string) UserSortOptions {
	normalizedSortBy := strings.ToLower(strings.TrimSpace(sortBy))
	normalizedSortOrder := strings.ToLower(strings.TrimSpace(sortOrder))
	if _, ok := userSortColumns[normalizedSortBy]; !ok {
		normalizedSortBy = "id"
		normalizedSortOrder = "desc"
	} else if normalizedSortOrder != "asc" {
		normalizedSortOrder = "desc"
	}

	return UserSortOptions{
		SortBy:    normalizedSortBy,
		SortOrder: normalizedSortOrder,
	}
}

func (options UserSortOptions) Apply(query *gorm.DB) *gorm.DB {
	columnName, ok := userSortColumns[options.SortBy]
	if !ok {
		columnName = "id"
	}
	q := query.Order(clause.OrderByColumn{
		Column: clause.Column{Name: columnName},
		Desc:   options.SortOrder != "asc",
	})
	if columnName != "id" {
		q = q.Order(clause.OrderByColumn{
			Column: clause.Column{Name: "id"},
			Desc:   true,
		})
	}
	return q
}

func resolveUserSortOptions(sortOptions []UserSortOptions) UserSortOptions {
	if len(sortOptions) == 0 {
		return NewUserSortOptions("", "")
	}
	return sortOptions[0]
}

// User if you add sensitive fields, don't forget to clean them in setupLogin function.
// Otherwise, the sensitive information will be saved on local storage in plain text!
type User struct {
	Id                   int                        `json:"id"`
	Username             string                     `json:"username" gorm:"unique;index" validate:"max=20"`
	Password             string                     `json:"password" gorm:"not null;" validate:"min=8,max=128"`
	HasPassword          bool                       `json:"-" gorm:"-:all"`
	OriginalPassword     string                     `json:"original_password" gorm:"-:all"` // this field is only for Password change verification, don't save it to database!
	DisplayName          string                     `json:"display_name" gorm:"index" validate:"max=20"`
	Role                 int                        `json:"role" gorm:"type:int;default:1"`   // admin, common
	Status               int                        `json:"status" gorm:"type:int;default:1"` // enabled, disabled
	Email                string                     `json:"email" gorm:"index" validate:"max=50"`
	GitHubId             string                     `json:"github_id" gorm:"column:github_id;index"`
	DiscordId            string                     `json:"discord_id" gorm:"column:discord_id;index"`
	OidcId               string                     `json:"oidc_id" gorm:"column:oidc_id;index"`
	WeChatId             string                     `json:"wechat_id" gorm:"column:wechat_id;index"`
	TelegramId           string                     `json:"telegram_id" gorm:"column:telegram_id;index"`
	VerificationCode     string                     `json:"verification_code" gorm:"-:all"`                         // this field is only for Email verification, don't save it to database!
	AccessToken          *string                    `json:"-" gorm:"type:char(32);column:access_token;uniqueIndex"` // this token is for system management
	AccessTokenCreatedAt *int64                     `json:"-" gorm:"type:bigint;column:access_token_created_at"`
	Quota                int                        `json:"quota" gorm:"type:int;default:0"`
	UsedQuota            int                        `json:"used_quota" gorm:"type:int;default:0;column:used_quota"` // used quota
	RequestCount         int                        `json:"request_count" gorm:"type:int;default:0;"`               // request number
	Group                string                     `json:"group" gorm:"type:varchar(64);default:'default'"`
	AffCode              string                     `json:"aff_code" gorm:"type:varchar(32);column:aff_code;uniqueIndex"`
	AffCount             int                        `json:"aff_count" gorm:"type:int;default:0;column:aff_count"`
	AffQuota             int                        `json:"aff_quota" gorm:"type:int;default:0;column:aff_quota"`           // 邀请剩余额度
	AffHistoryQuota      int                        `json:"aff_history_quota" gorm:"type:int;default:0;column:aff_history"` // 邀请历史额度
	InviterId            int                        `json:"inviter_id" gorm:"type:int;column:inviter_id;index"`
	DeletedAt            gorm.DeletedAt             `gorm:"index"`
	LinuxDOId            string                     `json:"linux_do_id" gorm:"column:linux_do_id;index"`
	Setting              string                     `json:"setting" gorm:"type:text;column:setting"`
	Remark               string                     `json:"remark,omitempty" gorm:"type:varchar(255)" validate:"max=255"`
	StripeCustomer       string                     `json:"stripe_customer" gorm:"type:varchar(64);column:stripe_customer;index"`
	CreatedAt            int64                      `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	LastLoginAt          int64                      `json:"last_login_at" gorm:"default:0;column:last_login_at"`
	AuthVersion          int64                      `json:"-" gorm:"type:bigint;not null;default:1;column:auth_version"`
	AdminPermissions     map[string]map[string]bool `json:"admin_permissions,omitempty" gorm:"-:all"`
}

func (user *User) ToBaseUser() *UserBase {
	cache := &UserBase{
		Id:          user.Id,
		Group:       user.Group,
		Quota:       user.Quota,
		Status:      user.Status,
		Role:        user.Role,
		Username:    user.Username,
		Setting:     user.Setting,
		Email:       user.Email,
		AuthVersion: user.AuthVersion,
		CacheSchema: userCacheSchemaVersion,
	}
	return cache
}

func (user *User) GetAccessToken() string {
	if user.AccessToken == nil {
		return ""
	}
	return *user.AccessToken
}

func (user *User) SetAccessToken(token string) {
	user.AccessToken = &token
}

// UpdateUserAccessToken rotates a dashboard personal access token without
// writing a stale user snapshot back over concurrently updated fields.
func UpdateUserAccessToken(id int, token string) error {
	if id == 0 {
		return errors.New("id 为空！")
	}
	result := DB.Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)}).Model(&User{}).Where("id = ?", id).Updates(map[string]any{
		"access_token": token, "access_token_created_at": common.GetTimestamp(),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// RevokeUserAccessToken returns the generation actually revoked under the row lock.
func RevokeUserAccessToken(id int) (string, error) {
	var tokenRef string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id", "access_token").First(&user, id).Error; err != nil {
			return err
		}
		tokenRef = AccessTokenFingerprint(user.GetAccessToken())
		if tokenRef == "" {
			return nil
		}
		return tx.Model(&User{}).Where("id = ?", id).Updates(map[string]any{"access_token": nil, "access_token_created_at": nil}).Error
	})
	return tokenRef, err
}

func (user *User) GetSetting() dto.UserSetting {
	setting := dto.UserSetting{}
	if user.Setting != "" {
		err := common.Unmarshal([]byte(user.Setting), &setting)
		if err != nil {
			common.SysLog("failed to unmarshal setting: " + err.Error())
		}
	}
	return setting
}

func (user *User) SetSetting(setting dto.UserSetting) {
	settingBytes, err := common.Marshal(setting)
	if err != nil {
		common.SysLog("failed to marshal setting: " + err.Error())
		return
	}
	user.Setting = string(settingBytes)
}

// userBindColumns 允许通过 UpdateUserBindColumn 更新的第三方账号绑定列白名单。
// 列名只可能来自代码内部的 provider 实现，白名单是防御纵深，不依赖调用方自律。
var userBindColumns = map[string]bool{
	"github_id":   true,
	"discord_id":  true,
	"oidc_id":     true,
	"linux_do_id": true,
	"wechat_id":   true,
}

// UpdateUserBindColumn 第三方账号绑定字段的专用更新。
// 绑定操作必须只写绑定列：若改为“读取完整用户 → 改一个字段 → 整体更新”，
// 读快照期间并发发生的封禁、降权或分组变更会被旧快照覆盖恢复。
// 角色、状态、分组只允许通过各自带锁/CAS 的专用方法修改。
func UpdateUserBindColumn(userId int, column string, value string) error {
	if userId <= 0 {
		return errors.New("id 为空！")
	}
	if !userBindColumns[column] {
		return fmt.Errorf("invalid user bind column: %s", column)
	}
	return DB.Model(&User{}).Where("id = ?", userId).Update(column, value).Error
}

// 根据用户角色生成默认的边栏配置
// CheckUserExistOrDeleted check if user exist or deleted, if not exist, return false, nil, if deleted or exist, return true, nil
func CheckUserExistOrDeleted(username string, email string) (bool, error) {
	var user User

	// err := DB.Unscoped().First(&user, "username = ? or email = ?", username, email).Error
	// check email if empty
	var err error
	email = NormalizeEmail(email)
	if email == "" {
		err = DB.Unscoped().First(&user, "username = ?", username).Error
	} else {
		err = DB.Unscoped().First(&user, "username = ? or LOWER(email) = ?", username, email).Error
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// not exist, return false, nil
			return false, nil
		}
		// other error, return false, err
		return false, err
	}
	// exist, return true, nil
	return true, nil
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func emailQuery(tx *gorm.DB, email string) *gorm.DB {
	if tx == nil {
		tx = DB
	}
	return tx.Unscoped().Model(&User{}).Where("LOWER(email) = ?", NormalizeEmail(email))
}

func CountUsersByEmail(email string) (int64, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return 0, nil
	}
	var count int64
	err := emailQuery(DB, email).Count(&count).Error
	return count, err
}

func IsEmailAvailable(email string, excludeUserID int) (bool, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return true, nil
	}
	query := emailQuery(DB, email)
	if excludeUserID > 0 {
		query = query.Where("id <> ?", excludeUserID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count == 0, nil
}

func EnsureEmailAvailable(email string, excludeUserID int) error {
	available, err := IsEmailAvailable(email, excludeUserID)
	if err != nil {
		return err
	}
	if !available {
		return ErrEmailAlreadyTaken
	}
	return nil
}

// withNormalizedEmailLock serializes concurrent writers that target the same
// normalized email inside tx, so a "check then write" sequence cannot be raced
// by two transactions. It must be called inside an active transaction; the lock
// is scoped to that transaction and released on commit/rollback.
//
//   - PostgreSQL: transaction-level advisory lock keyed by the normalized email.
//   - MySQL (default REPEATABLE READ): a locking read that takes a next-key/gap
//     lock on the email index, blocking concurrent inserts of the same value.
//   - SQLite: no explicit lock; the single-writer model already serializes the
//     write, so a racing second write fails instead of duplicating.
//
// An empty email is allowed to repeat and needs no serialization.
func withNormalizedEmailLock(tx *gorm.DB, email string, fn func(tx *gorm.DB) error) error {
	if err := lockNormalizedEmail(tx, email); err != nil {
		return err
	}
	return fn(tx)
}

func lockNormalizedEmail(tx *gorm.DB, email string) error {
	email = NormalizeEmail(email)
	if email == "" {
		return nil
	}
	switch {
	case common.UsingMainDatabase(common.DatabaseTypePostgreSQL):
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", email).Error; err != nil {
			return err
		}
	case common.UsingMainDatabase(common.DatabaseTypeMySQL):
		var ids []int
		if err := tx.Raw("SELECT id FROM users WHERE email = ? FOR UPDATE", email).Scan(&ids).Error; err != nil {
			return err
		}
	}
	return nil
}

func GetMaxUserId() int {
	var user User
	DB.Unscoped().Last(&user)
	return user.Id
}

// SearchInactiveUsers uses saved successful and failed invocation logs, rather
// than login timestamps. Filtering happens before pagination and works when
// the log database is separate from the account database.

// GetSelfUserById reads dashboard profile data and password existence in one
// query. The password hash and management access token are never selected.
func GetSelfUserById(id int) (*User, error) {
	return GetSelfUserByIdContext(context.Background(), id)
}

// GetSelfUserByIdContext keeps workspace reads cancellable and selects only
// profile fields and a computed password-existence flag.
func GetSelfUserByIdContext(ctx context.Context, id int) (*User, error) {
	if id <= 0 {
		return nil, errors.New("id 为空！")
	}
	var profile struct {
		User
		HasPassword bool `gorm:"column:has_password"`
	}
	err := DB.WithContext(ctx).Model(&User{}).Select([]string{
		"id", "username", "display_name", "role", "status", "email",
		"github_id", "discord_id", "oidc_id", "wechat_id", "telegram_id",
		"group", "quota", "used_quota", "request_count", "aff_code", "aff_count",
		"aff_quota", "aff_history", "inviter_id", "linux_do_id", "setting",
		"stripe_customer", "auth_version",
		"CASE WHEN password <> '' THEN 1 ELSE 0 END AS has_password",
	}).First(&profile, "id = ?", id).Error
	profile.User.HasPassword = profile.HasPassword
	return &profile.User, err
}

func GetUserIdByAffCode(affCode string) (int, error) {
	if affCode == "" {
		return 0, errors.New("affCode 为空！")
	}
	var user User
	err := DB.Select("id").First(&user, "aff_code = ?", affCode).Error
	return user.Id, err
}

func DeleteUserById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	user := User{Id: id}
	return user.Delete()
}

func HardDeleteUserById(id int) error {
	if id == 0 {
		return errors.New("id 为空！")
	}
	user := User{Id: id}
	return user.HardDelete()
}

func (user *User) prepareForInsert(tx *gorm.DB) error {
	user.Email = NormalizeEmail(user.Email)
	if err := ensureEmailAvailableWithTx(tx, user.Email, 0); err != nil {
		return err
	}
	if user.Password == "" {
		return nil
	}
	var err error
	user.Password, err = common.HashAccountPasswordContext(tx.Statement.Context, user.Password)
	return err
}

// BindEmailToUser atomically checks email availability and assigns it to the
// user, serializing concurrent binds of the same email so two accounts cannot
// end up sharing one address. The email is normalized before check and store.
func BindEmailToUser(user *User, email string) error {
	email = NormalizeEmail(email)
	if err := DB.Transaction(func(tx *gorm.DB) error {
		return withNormalizedEmailLock(tx, email, func(tx *gorm.DB) error {
			if err := ensureEmailAvailableWithTx(tx, email, user.Id); err != nil {
				return err
			}
			user.Email = email
			return user.UpdateWithTx(tx, false)
		})
	}); err != nil {
		return err
	}
	return updateUserCache(*user)
}

func ensureEmailAvailableWithTx(tx *gorm.DB, email string, excludeUserID int) error {
	email = NormalizeEmail(email)
	if email == "" {
		return nil
	}
	query := emailQuery(tx, email)
	if excludeUserID > 0 {
		query = query.Where("id <> ?", excludeUserID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrEmailAlreadyTaken
	}
	return nil
}

func (user *User) ClearBinding(bindingType string) error {
	if user.Id == 0 {
		return errors.New("user id is empty")
	}

	bindingColumnMap := map[string]string{
		"email":    "email",
		"github":   "github_id",
		"discord":  "discord_id",
		"oidc":     "oidc_id",
		"wechat":   "wechat_id",
		"telegram": "telegram_id",
		"linuxdo":  "linux_do_id",
	}

	column, ok := bindingColumnMap[bindingType]
	if !ok {
		return errors.New("invalid binding type")
	}

	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", user.Id).Update(column, "").Error; err != nil {
			return err
		}
		if bindingType == ExternalIdentityProviderTelegram {
			return ReleaseExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, user.Id)
		}
		if bindingType == ExternalIdentityProviderWeChat {
			return ReleaseExternalIdentityWithTx(tx, ExternalIdentityProviderWeChat, user.Id)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := DB.Where("id = ?", user.Id).First(user).Error; err != nil {
		return err
	}

	return updateUserCache(*user)
}

func (user *User) Delete() error {
	return user.delete(nil)
}

func DeleteUserForSession(identity AuthSessionIdentity) error {
	return DeleteUserForSessionContext(context.Background(), identity)
}

func DeleteUserForSessionContext(ctx context.Context, identity AuthSessionIdentity) error {
	user := User{Id: identity.UserID}
	return user.deleteContext(ctx, &identity)
}

func (user *User) delete(identity *AuthSessionIdentity) error {
	return user.deleteContext(context.Background(), identity)
}

func (user *User) deleteContext(ctx context.Context, identity *AuthSessionIdentity) error {
	return deleteAccountForIdentityContext(ctx, user, identity)
}
func (user *User) HardDelete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	var tokens []Token
	var deletedAuthVersion int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		deletedAuthVersion, err = IncrementUserAuthVersionWithTx(tx, user.Id)
		if err != nil {
			return err
		}
		if common.RedisEnabled {
			if err := tx.Unscoped().Select("id", commonKeyCol).Where("user_id = ?", user.Id).Find(&tokens).Error; err != nil {
				return err
			}
		}
		if err := deleteUserAuthenticationData(tx, user.Id); err != nil {
			return err
		}
		return tx.Unscoped().Delete(user).Error
	})
	if err != nil {
		return err
	}
	if err := publishCommittedUserAuthVersion(user.Id, deletedAuthVersion); err != nil {
		common.SysError(fmt.Sprintf("failed to publish auth tombstone after hard deleting user %d: %v", user.Id, err))
	}
	if err := invalidateTokensCache(tokens); err != nil {
		common.SysError(fmt.Sprintf("failed to invalidate token cache after hard deleting user %d: %v", user.Id, err))
	}
	if err := invalidateUserCache(user.Id); err != nil {
		common.SysError(fmt.Sprintf("failed to invalidate user cache after hard deleting user %d: %v", user.Id, err))
	}
	return nil
}

func deleteUserAuthenticationData(tx *gorm.DB, userId int) error {
	if err := releaseAllExternalIdentitiesWithTx(tx, userId); err != nil {
		return err
	}
	for _, authenticationData := range []any{
		&TwoFABackupCode{},
		&TwoFA{},
		&UserSession{},
		&AuthFlow{},
		&PasskeyCredential{},
		&Token{},
	} {
		if err := tx.Unscoped().Where("user_id = ?", userId).Delete(authenticationData).Error; err != nil {
			return err
		}
	}
	return deleteUserOAuthBindingsByUserId(tx, userId)
}

// ValidateAndFill check password & user status

func IsEmailAlreadyTaken(email string) bool {
	count, err := CountUsersByEmail(email)
	return err == nil && count > 0
}

func GetUniqueUserByEmail(email string) (*User, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return nil, ErrEmailNotFound
	}
	var users []User
	if err := DB.Where("LOWER(email) = ?", email).Limit(2).Find(&users).Error; err != nil {
		return nil, err
	}
	switch len(users) {
	case 0:
		return nil, ErrEmailNotFound
	case 1:
		return &users[0], nil
	default:
		return nil, ErrEmailAmbiguous
	}
}

func IsAdmin(userId int) bool {
	if userId == 0 {
		return false
	}
	var user User
	err := DB.Where("id = ?", userId).Select("role").Find(&user).Error
	if err != nil {
		common.SysLog("no such user " + err.Error())
		return false
	}
	return user.Role >= common.RoleAdminUser
}

// GetUserQuota gets quota from Redis first, falls back to DB if needed

// GetUserGroup gets group from Redis first, falls back to DB if needed

// GetUserSetting gets setting from Redis first, falls back to DB if needed

//func GetRootUserEmail() (email string) {
//	DB.Model(&User{}).Where("role = ?", common.RoleRootUser).Select("email").Find(&email)
//	return email
//}

// UpdateUserUsedQuota adjusts accumulated usage without changing request count.

// GetUsernameById gets username from Redis first, falls back to DB if needed
