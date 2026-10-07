package model

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dqt081228OHVGFY/further-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrSetupAlreadyCompleted = errors.New("系统已经初始化完成")
var bootstrapMutex sync.Mutex

type SetupInputError struct{ Message string }

func (e *SetupInputError) Error() string { return e.Message }

type BootstrapRequest struct {
	Username           string
	Password           string
	ConfirmPassword    string
	SelfUseModeEnabled bool
	DemoSiteEnabled    bool
}

func bootstrapAccount(request BootstrapRequest) (*User, error) {
	return bootstrapAccountContext(context.Background(), request)
}

func bootstrapAccountContext(ctx context.Context, request BootstrapRequest) (*User, error) {
	username := strings.TrimSpace(request.Username)
	if username == "" || utf8.RuneCountInString(username) > 20 || strings.ContainsFunc(username, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return nil, &SetupInputError{Message: "用户名应为 1 至 20 个字符，不能包含空白或控制字符"}
	}
	if request.Password != request.ConfirmPassword {
		return nil, &SetupInputError{Message: "两次输入的密码不一致"}
	}
	if err := common.ValidateNewAccountPassword(request.Password); err != nil {
		return nil, &SetupInputError{Message: err.Error()}
	}
	password, err := common.HashAccountPasswordContext(ctx, request.Password)
	if err != nil {
		return nil, err
	}
	return &User{Username: username, Password: password, Role: common.RoleRootUser, Status: common.UserStatusEnabled, DisplayName: "管理员", Quota: 100000000, AuthVersion: 1}, nil
}

// BootstrapGateway commits the owner, operating modes and completion marker
// together. ID 1 acts as a database-level setup claim across concurrent nodes.
// The process mutex also serializes in-memory publication after commit.
func BootstrapGateway(ctx context.Context, request BootstrapRequest) error {
	bootstrapMutex.Lock()
	defer bootstrapMutex.Unlock()
	if DB == nil {
		return errors.New("database is not initialized")
	}
	options := map[string]string{"SelfUseModeEnabled": strconv.FormatBool(request.SelfUseModeEnabled), "DemoSiteEnabled": strconv.FormatBool(request.DemoSiteEnabled)}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing Setup
		if err := tx.First(&existing).Error; err == nil {
			return ErrSetupAlreadyCompleted
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		claim := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Setup{ID: 1, Version: common.Version, InitializedAt: time.Now().Unix()})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return ErrSetupAlreadyCompleted
		}
		var owner User
		if err := tx.Where("role = ?", common.RoleRootUser).First(&owner).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			owner, err := bootstrapAccountContext(ctx, request)
			if err != nil {
				return err
			}
			if err := tx.Create(owner).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		for _, key := range []string{"SelfUseModeEnabled", "DemoSiteEnabled"} {
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&Option{Key: key, Value: options[key]}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()
	for _, key := range []string{"SelfUseModeEnabled", "DemoSiteEnabled"} {
		_ = updateOptionMap(key, options[key])
	}
	return nil
}
