package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type TokenDirectoryInput struct {
	OwnerID int
	Name    string
	Key     string
	Offset  int
	Limit   int
}

func validateLikePattern(pattern string) error {
	if strings.Contains(pattern, "%%") {
		return errors.New("搜索模式中不允许包含连续的 % 通配符")
	}
	count := strings.Count(pattern, "%")
	if count > 2 {
		return errors.New("搜索模式中最多允许包含 2 个 % 通配符")
	}
	if count > 0 && utf8.RuneCountInString(strings.ReplaceAll(pattern, "%", "")) < 2 {
		return errors.New("使用模糊搜索时，关键词长度至少为 2 个字符")
	}
	return nil
}

func sanitizeLikePattern(pattern string) (string, error) {
	if err := validateLikePattern(pattern); err != nil {
		return "", err
	}
	return strings.NewReplacer("!", "!!", "_", "!_").Replace(pattern), nil
}

func ListTokenDirectoryContext(ctx context.Context, input TokenDirectoryInput) ([]*Token, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if input.OwnerID <= 0 {
		return nil, 0, gorm.ErrRecordNotFound
	}
	input.Offset = max(0, input.Offset)
	if input.Limit <= 0 || input.Limit > searchHardLimit {
		input.Limit = searchHardLimit
	}
	input.Key = strings.TrimPrefix(input.Key, "sk-")
	private := DB.WithContext(ctx).Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)})
	if strings.Contains(input.Name, "%") || strings.Contains(input.Key, "%") {
		var count int64
		if err := private.Model(&Token{}).Where("user_id = ?", input.OwnerID).Count(&count).Error; err != nil {
			return nil, 0, err
		}
		if count > int64(operation_setting.GetMaxUserTokens()) {
			return nil, 0, errors.New("令牌数量超过上限，仅允许精确搜索，请勿使用 % 通配符")
		}
	}
	rows := make([]*Token, 0)
	var total int64
	err := private.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&Token{}).Where("user_id = ?", input.OwnerID)
		for _, field := range []struct{ column, value string }{{"name", input.Name}, {commonKeyCol, input.Key}} {
			if field.value == "" {
				continue
			}
			pattern, err := sanitizeLikePattern(field.value)
			if err != nil {
				return err
			}
			query = query.Where(field.column+" LIKE ? ESCAPE '!'", pattern)
		}
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return err
		}
		return query.Order("id DESC").Offset(input.Offset).Limit(input.Limit).Find(&rows).Error
	})
	return rows, total, err
}

func GetAllUserTokens(userID, offset, limit int) ([]*Token, error) {
	rows, _, err := ListTokenDirectoryContext(context.Background(), TokenDirectoryInput{OwnerID: userID, Offset: offset, Limit: limit})
	return rows, err
}

func SearchUserTokens(userID int, name, key string, offset, limit int) ([]*Token, int64, error) {
	return ListTokenDirectoryContext(context.Background(), TokenDirectoryInput{OwnerID: userID, Name: name, Key: key, Offset: offset, Limit: limit})
}

func GetTokenByIdsContext(ctx context.Context, id, ownerID int) (*Token, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if id <= 0 || ownerID <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var token Token
	if err := DB.WithContext(ctx).Where("id = ? AND user_id = ?", id, ownerID).First(&token).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

func GetTokenByIds(id, ownerID int) (*Token, error) {
	return GetTokenByIdsContext(context.Background(), id, ownerID)
}

func GetTokenById(id int) (*Token, error) {
	if id <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var token Token
	if err := DB.First(&token, id).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

func GetTokenByKey(key string, fromDB bool) (*Token, error) {
	return GetTokenByKeyContext(context.Background(), key, fromDB)
}

func ValidateUserTokenContext(ctx context.Context, key string) (*Token, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrTokenNotProvided
	}
	token, err := GetTokenByKeyContext(ctx, key, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenInvalid
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	if token.Status != common.TokenStatusEnabled {
		return token, ErrTokenInvalid
	}
	now := common.GetTimestamp()
	next := common.TokenStatusEnabled
	if token.ExpiredTime != -1 && token.ExpiredTime <= now {
		next = common.TokenStatusExpired
	}
	if next == common.TokenStatusEnabled && !token.UnlimitedQuota && token.RemainQuota <= 0 {
		next = common.TokenStatusExhausted
	}
	if next == common.TokenStatusEnabled {
		return token, nil
	}
	if !common.RedisEnabled {
		query := DB.WithContext(ctx).Model(&Token{}).Where("id = ? AND user_id = ? AND status = ?", token.Id, token.UserId, common.TokenStatusEnabled)
		if next == common.TokenStatusExpired {
			query = query.Where("expired_time = ? AND expired_time <= ?", token.ExpiredTime, now)
		} else {
			query = query.Where("unlimited_quota = ? AND remain_quota <= 0", false)
		}
		if err := query.Update("status", next).Error; err != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		token.Status = next
	}
	return token, ErrTokenInvalid
}

func ValidateUserToken(key string) (*Token, error) {
	return ValidateUserTokenContext(context.Background(), key)
}
