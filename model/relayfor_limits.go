package model

import (
	"errors"
	"sync/atomic"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var relayForAccessInstalled atomic.Pointer[gorm.DB]

func RelayForModelAccessInstalled() bool {
	if DB == nil {
		return false
	}
	if relayForAccessInstalled.Load() == DB {
		return true
	}
	if !DB.Migrator().HasTable(&RelayForModelAccess{}) {
		return false
	}
	relayForAccessInstalled.Store(DB)
	return true
}

// Access is opt-in per real model metadata record. A subscription listing a
// public model does not silently turn that model into a private model.
type RelayForModelAccess struct {
	ModelID          int    `json:"model_id" gorm:"primaryKey"`
	Model            string `json:"model" gorm:"-"`
	SubscriptionOnly bool   `json:"subscription_only"`
}

func ListRelayForModelAccesses() ([]RelayForModelAccess, error) {
	var entries []RelayForModelAccess
	if err := DB.Find(&entries).Error; err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ModelID)
	}
	result := make([]RelayForModelAccess, 0, len(entries))
	if len(ids) == 0 {
		return result, nil
	}
	var metadata []Model
	if err := DB.Select("id", "model_name").Where("id IN ?", ids).Find(&metadata).Error; err != nil {
		return nil, err
	}
	names := map[int]string{}
	for _, entry := range metadata {
		names[entry.Id] = entry.ModelName
	}
	for _, entry := range entries {
		if name, exists := names[entry.ModelID]; exists {
			entry.Model = name
			result = append(result, entry)
		}
	}
	return result, nil
}

func GetRelayForModelAccess(modelID int) (RelayForModelAccess, error) {
	var metadata Model
	if err := DB.First(&metadata, modelID).Error; err != nil {
		return RelayForModelAccess{}, err
	}
	access := RelayForModelAccess{ModelID: modelID, Model: metadata.ModelName}
	err := DB.First(&access, "model_id = ?", modelID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	access.Model = metadata.ModelName
	return access, err
}

func SaveRelayForModelAccess(access RelayForModelAccess) (RelayForModelAccess, error) {
	current, err := GetRelayForModelAccess(access.ModelID)
	if err != nil {
		return current, err
	}
	if access.Model != "" && access.Model != current.Model {
		return current, errors.New("模型名称与模型 ID 不一致")
	}
	access.Model = current.Model
	err = DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "model_id"}}, DoUpdates: clause.AssignmentColumns([]string{"subscription_only"})}).Create(&access).Error
	return access, err
}

func RelayForSubscriptionModelAllowed(userID int, modelName string) (bool, error) {
	if modelName == "" || !RelayForModelAccessInstalled() {
		return true, nil
	}
	var configured []Model
	if err := DB.Model(&Model{}).
		Joins("JOIN relay_for_model_accesses AS access ON access.model_id = models.id").
		Where("access.subscription_only = ? AND (models.model_name = ? OR models.name_rule <> ?)", true, modelName, NameRuleExact).
		Find(&configured).Error; err != nil {
		return false, err
	}
	var exclusive []Model
	for _, metadata := range configured {
		if metadata.MatchesName(modelName) {
			exclusive = append(exclusive, metadata)
		}
	}
	if len(exclusive) == 0 {
		return true, nil
	}
	entitlements, err := GetRelayForActiveEntitlements(userID, modelName)
	if err != nil {
		return false, err
	}
	for _, entitlement := range entitlements {
		for _, rule := range entitlement.Policy.Rules() {
			for _, restricted := range exclusive {
				if rule.ModelID == restricted.Id {
					return true, nil
				}
			}
			match := Model{ModelName: rule.Model, NameRule: rule.NameRule}
			if rule.Model != "" && match.MatchesName(modelName) {
				return true, nil
			}
		}
	}
	return false, nil
}
