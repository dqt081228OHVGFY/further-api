package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func readLedgerInteger(values map[string]string, field string) (int64, error) {
	value, exists := values[field]
	if !exists {
		return 0, fmt.Errorf("ledger cache lacks %s", field)
	}
	return strconv.ParseInt(value, 10, 64)
}

func userAuthFloorContext(ctx context.Context, userID int) (int64, error) {
	values, err := common.RDB.MGet(ctx, getUserAuthFenceKey(userID), getUserAuthVersionKey(userID)).Result()
	if err != nil {
		return 0, err
	}
	var floor int64
	for _, value := range values {
		if value == nil {
			continue
		}
		version, err := strconv.ParseInt(fmt.Sprint(value), 10, 64)
		if err != nil {
			return 0, err
		}
		floor = max(floor, version)
	}
	return floor, nil
}

func hydrateLedgerUserContext(ctx context.Context, user *User) error {
	const initialize = `
local incoming = tonumber(ARGV[1])
local floor = math.max(tonumber(redis.call('GET', KEYS[2]) or '0'), tonumber(redis.call('GET', KEYS[3]) or '0'))
local current = tonumber(redis.call('HGET', KEYS[1], 'AuthVersion') or '0')
if floor > incoming or current > incoming then return 0 end
redis.call('HSET', KEYS[1], 'Id', ARGV[2], 'AuthVersion', ARGV[1], 'CacheSchema', ARGV[3], 'Group', ARGV[4], 'Email', ARGV[5], 'Status', ARGV[6], 'Role', ARGV[7], 'Username', ARGV[8], 'Setting', ARGV[9])
if redis.call('HEXISTS', KEYS[1], 'Quota') == 0 then redis.call('HSET', KEYS[1], 'Quota', ARGV[10]) end
if tonumber(redis.call('GET', KEYS[3]) or '0') < incoming then redis.call('SET', KEYS[3], ARGV[1]) end
if tonumber(redis.call('GET', KEYS[2]) or '0') <= incoming then redis.call('DEL', KEYS[2]) end
redis.call('EXPIRE', KEYS[1], ARGV[11])
return 1`
	result, err := common.RDB.Eval(ctx, initialize, []string{getUserCacheKey(user.Id), getUserAuthFenceKey(user.Id), getUserAuthVersionKey(user.Id)}, user.AuthVersion, user.Id, userCacheSchemaVersion, user.Group, user.Email, user.Status, user.Role, user.Username, user.Setting, user.Quota, userCacheTTLSeconds()).Int()
	if err == nil && result == 0 {
		return ErrUserAuthCachePending
	}
	return err
}

func GetUserCacheContext(ctx context.Context, userID int) (*UserBase, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if userID <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	redisActive := common.RedisEnabled && common.RDB != nil
	if redisActive {
		values, err := common.RDB.HGetAll(ctx, getUserCacheKey(userID)).Result()
		if err == nil && len(values) > 0 {
			integers := make(map[string]int64)
			for _, field := range []string{"Id", "Quota", "Status", "Role", "AuthVersion", "CacheSchema"} {
				integers[field], err = readLedgerInteger(values, field)
				if err != nil {
					break
				}
			}
			if err == nil && integers["Id"] == int64(userID) && integers["CacheSchema"] == userCacheSchemaVersion && integers["AuthVersion"] > 0 {
				floor, err := userAuthFloorContext(ctx, userID)
				if err == nil && floor <= integers["AuthVersion"] {
					return &UserBase{Id: userID, Quota: int(integers["Quota"]), Status: int(integers["Status"]), Role: int(integers["Role"]), AuthVersion: integers["AuthVersion"], CacheSchema: userCacheSchemaVersion, Group: values["Group"], Email: values["Email"], Username: values["Username"], Setting: values["Setting"]}, nil
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var user User
	if err := DB.WithContext(ctx).Omit("password", "access_token").First(&user, userID).Error; err != nil {
		return nil, err
	}
	if redisActive {
		floor, err := userAuthFloorContext(ctx, userID)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err == nil && floor > user.AuthVersion {
			return nil, ErrUserAuthCachePending
		}
		if err := hydrateLedgerUserContext(ctx, &user); errors.Is(err, ErrUserAuthCachePending) {
			return nil, err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return user.ToBaseUser(), nil
}

func GetUserQuotaContext(ctx context.Context, id int, fromDB bool) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if !fromDB && common.RedisEnabled {
		user, err := GetUserCacheContext(ctx, id)
		if err != nil {
			return 0, err
		}
		return user.Quota, nil
	}
	var user User
	err := DB.WithContext(ctx).Select("id", "quota").First(&user, id).Error
	return user.Quota, err
}

func ledgerTokenSnapshotContext(ctx context.Context, key string) (*Token, error) {
	values, err := common.RDB.HGetAll(ctx, getTokenCacheKey(key)).Result()
	if err != nil {
		return nil, err
	}
	var token Token
	for _, field := range []struct {
		name        string
		destination *int
	}{
		{"Id", &token.Id}, {"UserId", &token.UserId}, {"Status", &token.Status}, {"RemainQuota", &token.RemainQuota}, {"UsedQuota", &token.UsedQuota},
	} {
		value, err := readLedgerInteger(values, field.name)
		if err != nil {
			return nil, err
		}
		*field.destination = int(value)
	}
	for _, field := range []struct {
		name        string
		destination *int64
	}{
		{"CreatedTime", &token.CreatedTime}, {"AccessedTime", &token.AccessedTime}, {"ExpiredTime", &token.ExpiredTime},
	} {
		value, err := readLedgerInteger(values, field.name)
		if err != nil {
			return nil, err
		}
		*field.destination = value
	}
	for _, field := range []struct {
		name        string
		destination *bool
	}{
		{"UnlimitedQuota", &token.UnlimitedQuota}, {"ModelLimitsEnabled", &token.ModelLimitsEnabled}, {"CrossGroupRetry", &token.CrossGroupRetry},
	} {
		value, err := strconv.ParseBool(values[field.name])
		if err != nil {
			return nil, err
		}
		*field.destination = value
	}
	if token.Id <= 0 || token.UserId <= 0 {
		return nil, ErrTokenInvalid
	}
	if fenced, err := common.RDB.Exists(ctx, getTokenCacheFenceKey(key)).Result(); err != nil || fenced != 0 {
		return nil, errors.New("token metadata cache is fenced")
	}
	allow := values["AllowIps"]
	token.Key, token.Name, token.ModelLimits, token.Group, token.AutoGroups, token.AllowIps = key, values["Name"], values["ModelLimits"], values["Group"], values["AutoGroups"], &allow
	return &token, nil
}

func hydrateLedgerTokenContext(ctx context.Context, token *Token) error {
	const initialize = `
if redis.call('EXISTS', KEYS[2]) == 1 then return 0 end
if redis.call('EXISTS', KEYS[1]) == 0 then
  for index = 2, #ARGV, 2 do redis.call('HSET', KEYS[1], ARGV[index], ARGV[index+1]) end
end
redis.call('EXPIRE', KEYS[1], ARGV[1])
return 1`
	allow := ""
	if token.AllowIps != nil {
		allow = *token.AllowIps
	}
	args := []any{tokenCacheTTLSeconds(), "Id", token.Id, "UserId", token.UserId, "Status", token.Status, "Name", token.Name, "CreatedTime", token.CreatedTime, "AccessedTime", token.AccessedTime, "ExpiredTime", token.ExpiredTime, "RemainQuota", token.RemainQuota, "UsedQuota", token.UsedQuota, "UnlimitedQuota", strconv.FormatBool(token.UnlimitedQuota), "ModelLimitsEnabled", strconv.FormatBool(token.ModelLimitsEnabled), "ModelLimits", token.ModelLimits, "AllowIps", allow, "Group", token.Group, "CrossGroupRetry", strconv.FormatBool(token.CrossGroupRetry), "AutoGroups", token.AutoGroups}
	return common.RDB.Eval(ctx, initialize, []string{getTokenCacheKey(token.Key), getTokenCacheFenceKey(token.Key)}, args...).Err()
}

func GetTokenByKeyContext(ctx context.Context, key string, fromDB bool) (*Token, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if key == "" {
		return nil, ErrTokenInvalid
	}
	redisActive := common.RedisEnabled && common.RDB != nil
	if redisActive && !fromDB {
		if token, err := ledgerTokenSnapshotContext(ctx, key); err == nil {
			return token, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var token Token
	if err := DB.WithContext(ctx).Session(&gorm.Session{Logger: DB.Logger.LogMode(gormlogger.Silent)}).Where(commonKeyCol+" = ?", key).First(&token).Error; err != nil {
		return nil, err
	}
	if redisActive {
		_ = hydrateLedgerTokenContext(ctx, &token)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &token, nil
}

type ledgerReservationInput struct {
	id               int
	key              string
	token, unlimited bool
	amount           int
}

func applyLedgerDeltaContext(ctx context.Context, input ledgerReservationInput, delta int, batching bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if batching && common.BatchUpdateEnabled {
		kind := BatchUpdateTypeUserQuota
		if input.token {
			kind = BatchUpdateTypeTokenQuota
		}
		addNewRecord(kind, input.id, delta)
		return nil
	}
	var result *gorm.DB
	if input.token {
		result = DB.WithContext(ctx).Model(&Token{}).Where("id = ?", input.id).Updates(map[string]any{"remain_quota": gorm.Expr("remain_quota + ?", delta), "used_quota": gorm.Expr("used_quota - ?", delta), "accessed_time": common.GetTimestamp()})
	} else {
		result = DB.WithContext(ctx).Model(&User{}).Where("id = ?", input.id).Update("quota", gorm.Expr("quota + ?", delta))
	}
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func reserveLedgerDatabaseContext(ctx context.Context, input ledgerReservationInput) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	var result *gorm.DB
	if input.token {
		query := DB.WithContext(ctx).Model(&Token{}).Where("id = ?", input.id)
		if !input.unlimited {
			query = query.Where("remain_quota >= ?", input.amount)
		}
		result = query.Updates(map[string]any{"remain_quota": gorm.Expr("remain_quota - ?", input.amount), "used_quota": gorm.Expr("used_quota + ?", input.amount), "accessed_time": common.GetTimestamp()})
	} else {
		result = DB.WithContext(ctx).Model(&User{}).Where("id = ? AND quota >= ?", input.id, input.amount).Update("quota", gorm.Expr("quota - ?", input.amount))
	}
	return result.RowsAffected == 1, result.Error
}

func mutateLedgerCacheContext(ctx context.Context, input ledgerReservationInput, delta int, reserve bool) (cacheQuotaResult, error) {
	script, key, args := userQuotaDeltaScript, getUserCacheKey(input.id), []any{delta, input.id, userCacheSchemaVersion}
	if reserve {
		script, args = userQuotaReserveScript, []any{input.amount, input.id, userCacheSchemaVersion}
	}
	if input.token {
		script, key, args = tokenQuotaDeltaScript, getTokenCacheKey(input.key), []any{delta, input.id, common.GetTimestamp()}
		if reserve && !input.unlimited {
			script, args = tokenQuotaReserveScript, []any{input.amount, input.id, common.GetTimestamp()}
		}
	}
	value, err := common.RDB.Eval(ctx, script, []string{key}, args...).Int()
	return quotaResultFromLua(value, err)
}

func reserveLedgerContext(ctx context.Context, input ledgerReservationInput) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if input.amount < 0 || input.amount > common.MaxWalletQuota {
		return false, errors.New("预扣额度无效")
	}
	if input.amount == 0 {
		return true, nil
	}
	if input.id <= 0 {
		return false, gorm.ErrRecordNotFound
	}
	if !common.RedisEnabled || common.RDB == nil {
		return reserveLedgerDatabaseContext(ctx, input)
	}
	result, err := mutateLedgerCacheContext(ctx, input, -input.amount, true)
	if err == nil && result == cacheQuotaMiss {
		if input.token {
			_, err = GetTokenByKeyContext(ctx, input.key, true)
		} else {
			_, err = GetUserCacheContext(ctx, input.id)
		}
		if err == nil {
			result, err = mutateLedgerCacheContext(ctx, input, -input.amount, true)
		}
	}
	if ctx.Err() != nil {
		if err == nil && result == cacheQuotaOK {
			compensation, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, compensateErr := mutateLedgerCacheContext(compensation, input, input.amount, false)
			return false, errors.Join(ctx.Err(), compensateErr)
		}
		return false, ctx.Err()
	}
	if errors.Is(err, ErrUserAuthCachePending) {
		return false, err
	}
	if err != nil || result == cacheQuotaMiss {
		return reserveLedgerDatabaseContext(ctx, input)
	}
	if result == cacheQuotaInsufficient {
		return false, nil
	}
	if err := applyLedgerDeltaContext(ctx, input, -input.amount, true); err != nil {
		compensation, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, compensateErr := mutateLedgerCacheContext(compensation, input, input.amount, false)
		return false, errors.Join(err, compensateErr, ctx.Err())
	}
	return true, nil
}

func TryReserveUserQuotaContext(ctx context.Context, id, quota int) (bool, error) {
	return reserveLedgerContext(ctx, ledgerReservationInput{id: id, amount: quota})
}

func TryReserveTokenQuotaContext(ctx context.Context, id int, key string, quota int, unlimited bool) (bool, error) {
	return reserveLedgerContext(ctx, ledgerReservationInput{id: id, key: key, amount: quota, token: true, unlimited: unlimited})
}

func IncreaseTokenQuotaContext(ctx context.Context, id int, key string, quota int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if quota < 0 {
		return errors.New("退款额度无效")
	}
	if quota == 0 {
		return nil
	}
	input := ledgerReservationInput{id: id, key: key, token: true}
	if err := applyLedgerDeltaContext(ctx, input, quota, true); err != nil {
		return err
	}
	if common.RedisEnabled && common.RDB != nil {
		_, _ = mutateLedgerCacheContext(ctx, input, quota, false)
	}
	return nil
}

func IncreaseUserQuotaContext(ctx context.Context, id, quota int, syncDB bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if quota < 0 || quota > common.MaxWalletQuota {
		return errors.New("入账额度无效")
	}
	if quota == 0 {
		return nil
	}
	input := ledgerReservationInput{id: id}
	if common.BatchUpdateEnabled && !syncDB {
		if err := applyLedgerDeltaContext(ctx, input, quota, true); err != nil {
			return err
		}
	} else {
		result := DB.WithContext(ctx).Model(&User{}).Where("id = ? AND quota <= ?", id, common.MaxWalletQuota-quota).Update("quota", gorm.Expr("quota + ?", quota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			var count int64
			if err := DB.WithContext(ctx).Model(&User{}).Where("id = ?", id).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return gorm.ErrRecordNotFound
			}
			return ErrWalletQuotaLimitExceeded
		}
	}
	if common.RedisEnabled && common.RDB != nil {
		_, _ = mutateLedgerCacheContext(ctx, input, quota, false)
	}
	return nil
}
