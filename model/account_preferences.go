package model

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"gorm.io/gorm"
)

func MutateAccountPreferences(ctx context.Context, userID int, mutate func(*User, *dto.UserSetting) error) error {
	if userID <= 0 || mutate == nil {
		return errors.New("用户设置参数无效")
	}
	var encoded string
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var account User
		if err := lockForUpdate(tx).First(&account, userID).Error; err != nil {
			return err
		}
		preferences := account.GetSetting()
		welcomeCompleted := preferences.WelcomeCompleted
		if err := mutate(&account, &preferences); err != nil {
			return err
		}
		preferences.WelcomeCompleted = welcomeCompleted || preferences.WelcomeCompleted
		data, err := common.Marshal(preferences)
		if err != nil {
			return err
		}
		encoded = string(data)
		return tx.Model(&User{}).Where("id = ?", userID).Update("setting", encoded).Error
	})
	if err != nil {
		return err
	}
	return updateUserSettingCache(userID, encoded)
}

func UpdateUserSetting(userID int, preferences dto.UserSetting) error {
	return MutateAccountPreferences(context.Background(), userID, func(_ *User, current *dto.UserSetting) error {
		*current = preferences
		return nil
	})
}

type SelfAccountProfileInput struct {
	Username    *string
	DisplayName *string
}

func UpdateSelfAccountProfile(ctx context.Context, userID int, input SelfAccountProfileInput) error {
	if userID <= 0 {
		return gorm.ErrRecordNotFound
	}
	updates := make(map[string]any)
	if input.Username != nil {
		username := strings.TrimSpace(*input.Username)
		if length := utf8.RuneCountInString(username); length == 0 || length > UserNameMaxLength {
			return errors.New("用户名需为 1 至 20 个字符")
		}
		updates["username"] = username
	}
	if input.DisplayName != nil {
		if utf8.RuneCountInString(*input.DisplayName) > 20 {
			return errors.New("显示名称不能超过 20 个字符")
		}
		updates["display_name"] = *input.DisplayName
	}
	if len(updates) == 0 {
		return errors.New("请提供要修改的账户信息")
	}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var account User
		if err := lockForUpdate(tx).Select("id").First(&account, userID).Error; err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", userID).Updates(updates).Error
	})
	if err != nil {
		return err
	}
	return PublishUserAuthCache(userID)
}
