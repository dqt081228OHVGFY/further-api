package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type AccountDirectoryInput struct {
	Keyword       string
	Group         string
	Role          *int
	Status        *int
	Offset        int
	Limit         int
	InactiveSince int64
	Sort          UserSortOptions
}

func accountDirectoryScope(db *gorm.DB, input AccountDirectoryInput) *gorm.DB {
	query := db.Unscoped().Model(&User{})
	if input.Keyword != "" {
		pattern := "%" + input.Keyword + "%"
		condition := "username LIKE ? OR email LIKE ? OR display_name LIKE ?"
		arguments := []any{pattern, pattern, pattern}
		if id, err := strconv.Atoi(input.Keyword); err == nil {
			condition = "id = ? OR " + condition
			arguments = append([]any{id}, arguments...)
		}
		query = query.Where("("+condition+")", arguments...)
	}
	if input.Group != "" {
		query = query.Where(commonGroupCol+" = ?", input.Group)
	}
	if input.Role != nil {
		query = query.Where("role = ?", *input.Role)
	}
	if input.Status != nil {
		if *input.Status == -1 {
			query = query.Where("deleted_at IS NOT NULL")
		} else {
			query = query.Where("deleted_at IS NULL AND status = ?", *input.Status)
		}
	}
	return query
}

func ListAccountDirectoryContext(ctx context.Context, input AccountDirectoryInput) ([]*User, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	input.Offset = max(input.Offset, 0)
	users := make([]*User, 0)
	var total int64
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := accountDirectoryScope(tx, input)
		if input.InactiveSince > 0 {
			if LOG_DB == nil {
				return errors.New("调用日志数据库暂不可用")
			}
			if LOG_DB == DB {
				active := tx.Model(&Log{}).Select("user_id").Where("created_at >= ? AND type IN ?", input.InactiveSince, []int{LogTypeConsume, LogTypeError})
				query = query.Where("id NOT IN (?)", active)
			} else {
				// Keep only the requested page, rather than collecting every
				// account ID in memory when the activity database is separate.
				selected := make([]int, 0)
				for cursor := 0; ; cursor += 500 {
					var ids []int
					if err := input.Sort.Apply(query.Session(&gorm.Session{})).Offset(cursor).Limit(500).Pluck("id", &ids).Error; err != nil {
						return err
					}
					if len(ids) == 0 {
						break
					}
					var active []int
					if err := LOG_DB.WithContext(ctx).Model(&Log{}).Where("user_id IN ? AND created_at >= ? AND type IN ?", ids, input.InactiveSince, []int{LogTypeConsume, LogTypeError}).Distinct("user_id").Pluck("user_id", &active).Error; err != nil {
						return err
					}
					seen := make(map[int]bool, len(active))
					for _, id := range active {
						seen[id] = true
					}
					for _, id := range ids {
						if seen[id] {
							continue
						}
						if total >= int64(input.Offset) && (input.Limit < 0 || len(selected) < input.Limit) {
							selected = append(selected, id)
						}
						total++
					}
					if len(ids) < 500 {
						break
					}
				}
				if len(selected) == 0 {
					return nil
				}
				return input.Sort.Apply(query.Where("id IN ?", selected)).Omit("password", "access_token").Find(&users).Error
			}
		}
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return err
		}
		return input.Sort.Apply(query).Omit("password", "access_token").Offset(input.Offset).Limit(input.Limit).Find(&users).Error
	})
	return users, total, err
}

func GetAllUsers(page *common.PageInfo, sort ...UserSortOptions) ([]*User, int64, error) {
	return ListAccountDirectoryContext(context.Background(), AccountDirectoryInput{Offset: page.GetStartIdx(), Limit: page.GetPageSize(), Sort: resolveUserSortOptions(sort)})
}

func SearchUsers(keyword, group string, role, status *int, offset, limit int, sort ...UserSortOptions) ([]*User, int64, error) {
	return ListAccountDirectoryContext(context.Background(), AccountDirectoryInput{Keyword: keyword, Group: group, Role: role, Status: status, Offset: offset, Limit: limit, Sort: resolveUserSortOptions(sort)})
}

func SearchInactiveUsers(keyword, group string, role, status *int, offset, limit int, since int64, sort ...UserSortOptions) ([]*User, int64, error) {
	if since <= 0 {
		return nil, 0, errors.New("无效的调用活跃时间范围")
	}
	return ListAccountDirectoryContext(context.Background(), AccountDirectoryInput{Keyword: keyword, Group: group, Role: role, Status: status, Offset: offset, Limit: limit, InactiveSince: since, Sort: resolveUserSortOptions(sort)})
}

func GetUserByIdContext(ctx context.Context, id int, includeCredentials bool) (*User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	query := DB.WithContext(ctx).Model(&User{})
	if !includeCredentials {
		query = query.Omit("password", "access_token")
	}
	var account User
	if err := query.First(&account, id).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func GetUserById(id int, includeCredentials bool) (*User, error) {
	return GetUserByIdContext(context.Background(), id, includeCredentials)
}

func AuthenticatePasswordContext(ctx context.Context, identifier, password string) (*User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || password == "" {
		return nil, ErrUserEmptyCredentials
	}
	var account User
	private := DB.WithContext(ctx).Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)})
	// Username takes precedence; an email collision never chooses an unrelated
	// lower-ID account through a mixed OR query.
	err := private.Where("username = ?", identifier).First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var matches []User
		err = private.Where("LOWER(email) = ?", NormalizeEmail(identifier)).Limit(2).Find(&matches).Error
		if err == nil && len(matches) == 1 {
			account = matches[0]
		} else if err == nil {
			return nil, ErrInvalidCredentials
		}
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	if account.Password == "" || account.Status != common.UserStatusEnabled {
		return nil, ErrInvalidCredentials
	}
	valid, err := common.ValidatePasswordAndHashContext(ctx, password, account.Password)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, ErrInvalidCredentials
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &account, nil
}

func (user *User) ValidateAndFill() error {
	account, err := AuthenticatePasswordContext(context.Background(), user.Username, user.Password)
	if err == nil {
		*user = *account
	}
	return err
}

func LookupExternalAccountContext(ctx context.Context, column, identity string) (*User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	allowed := map[string]bool{"github_id": true, "discord_id": true, "oidc_id": true, "wechat_id": true, "telegram_id": true, "linux_do_id": true, "email": true}
	if !allowed[column] || identity == "" {
		return nil, errors.New("外部账户标识无效")
	}
	var accounts []User
	query := DB.WithContext(ctx).Unscoped().Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)}).Where(column+" = ?", identity)
	if column == "email" {
		query = DB.WithContext(ctx).Unscoped().Where("LOWER(email) = ?", NormalizeEmail(identity))
	}
	if err := query.Limit(2).Find(&accounts).Error; err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if len(accounts) > 1 {
		return nil, ErrEmailAmbiguous
	}
	return &accounts[0], nil
}

func fillExternalAccount(user *User, column, identity string) error {
	account, err := LookupExternalAccountContext(context.Background(), column, identity)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		user.Id = 0
		return nil
	}
	if err != nil {
		return err
	}
	if account.DeletedAt.Valid {
		user.Id = 0
		return nil
	}
	*user = *account
	return nil
}

func (user *User) FillUserById() error {
	account, err := GetUserByIdContext(context.Background(), user.Id, true)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		user.Id = 0
		return nil
	}
	if err == nil {
		*user = *account
	}
	return err
}
func (user *User) FillUserByEmail() error { return fillExternalAccount(user, "email", user.Email) }
func (user *User) FillUserByGitHubId() error {
	return fillExternalAccount(user, "github_id", user.GitHubId)
}
func (user *User) FillUserByDiscordId() error {
	return fillExternalAccount(user, "discord_id", user.DiscordId)
}
func (user *User) FillUserByOidcId() error { return fillExternalAccount(user, "oidc_id", user.OidcId) }
func (user *User) FillUserByWeChatId() error {
	return fillExternalAccount(user, "wechat_id", user.WeChatId)
}
func (user *User) FillUserByLinuxDOId() error {
	return fillExternalAccount(user, "linux_do_id", user.LinuxDOId)
}

func (user *User) FillUserByTelegramId() error {
	account, err := LookupExternalAccountContext(context.Background(), "telegram_id", user.TelegramId)
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && account.DeletedAt.Valid {
		return errors.New("该 Telegram 账户未绑定")
	}
	if err == nil {
		*user = *account
	}
	return err
}

func externalAccountTaken(column, identity string) bool {
	_, err := LookupExternalAccountContext(context.Background(), column, identity)
	return err == nil || errors.Is(err, ErrEmailAmbiguous)
}
func IsWeChatIdAlreadyTaken(id string) bool   { return externalAccountTaken("wechat_id", id) }
func IsGitHubIdAlreadyTaken(id string) bool   { return externalAccountTaken("github_id", id) }
func IsDiscordIdAlreadyTaken(id string) bool  { return externalAccountTaken("discord_id", id) }
func IsOidcIdAlreadyTaken(id string) bool     { return externalAccountTaken("oidc_id", id) }
func IsTelegramIdAlreadyTaken(id string) bool { return externalAccountTaken("telegram_id", id) }
func IsLinuxDOIdAlreadyTaken(id string) bool  { return externalAccountTaken("linux_do_id", id) }

func ValidateAccessTokenContext(ctx context.Context, token string) (*User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if token == "" {
		return nil, nil
	}
	token = strings.TrimPrefix(token, "Bearer ")
	var user User
	err := DB.WithContext(ctx).Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)}).Where("access_token = ?", token).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	return &user, nil
}

func ValidateAccessToken(token string) (*User, error) {
	return ValidateAccessTokenContext(context.Background(), token)
}
