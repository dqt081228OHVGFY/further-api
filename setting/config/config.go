package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type ConfigManager struct {
	configs map[string]any
	mutex   sync.RWMutex
}

var GlobalConfig = NewConfigManager()

func NewConfigManager() *ConfigManager {
	return &ConfigManager{configs: make(map[string]any)}
}

func (cm *ConfigManager) Register(name string, config any) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	cm.configs[name] = config
}

func (cm *ConfigManager) Get(name string) any {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	return cm.configs[name]
}

func (cm *ConfigManager) ValidateUpdate(name string, fields map[string]string) error {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	if cfg, ok := cm.configs[name]; ok {
		_, err := prepareUpdate(cfg, fields)
		return err
	}
	return fmt.Errorf("unknown configuration module %q", name)
}

func (cm *ConfigManager) Update(name string, fields map[string]string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	if cfg, ok := cm.configs[name]; ok {
		return UpdateConfigFromMap(cfg, fields)
	}
	return fmt.Errorf("unknown configuration module %q", name)
}

// LoadFromDB stages every module first. A corrupt stored value must not leave
// half of a configuration generation active in the running process.
func (cm *ConfigManager) LoadFromDB(options map[string]string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	var updates []*configUpdate
	var failures []error
	for _, name := range sortedKeys(cm.configs) {
		fields := make(map[string]string)
		for key, value := range options {
			if field, ok := strings.CutPrefix(key, name+"."); ok {
				fields[field] = value
			}
		}
		if len(fields) == 0 {
			continue
		}
		update, err := prepareUpdate(cm.configs[name], fields)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", name, err))
			continue
		}
		updates = append(updates, update)
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	for _, update := range updates {
		if err := update.commit(); err != nil {
			return err
		}
	}
	return nil
}

// Snapshot before invoking callbacks: a persistence callback may read or
// update the registry, and must never run while its mutex is held.
func (cm *ConfigManager) SaveToDB(updateFunc func(key, value string) error) error {
	if updateFunc == nil {
		return fmt.Errorf("configuration persistence callback is nil")
	}
	cm.mutex.RLock()
	snapshot, err := cm.export()
	cm.mutex.RUnlock()
	if err != nil {
		return err
	}
	for _, key := range sortedKeys(snapshot) {
		if err := updateFunc(key, snapshot[key]); err != nil {
			return fmt.Errorf("persist configuration %s: %w", key, err)
		}
	}
	return nil
}

func (cm *ConfigManager) ExportAllConfigs() map[string]string {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	result, _ := cm.export()
	return result
}

func (cm *ConfigManager) export() (map[string]string, error) {
	result := make(map[string]string)
	for _, name := range sortedKeys(cm.configs) {
		fields, err := ConfigToMap(cm.configs[name])
		if err != nil {
			return nil, fmt.Errorf("export configuration %s: %w", name, err)
		}
		for key, value := range fields {
			result[name+"."+key] = value
		}
	}
	return result, nil
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func fieldName(field reflect.StructField) (string, bool) {
	if !field.IsExported() {
		return "", false
	}
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name == "-" {
		return "", false
	}
	if name == "" {
		name = field.Name
	}
	return name, true
}

func structValue(config any, writable bool) (reflect.Value, error) {
	value := reflect.ValueOf(config)
	if !value.IsValid() {
		return reflect.Value{}, fmt.Errorf("configuration must be a struct")
	}
	if writable && (value.Kind() != reflect.Pointer || value.IsNil()) {
		return reflect.Value{}, fmt.Errorf("configuration update requires a non-nil struct pointer")
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return reflect.Value{}, fmt.Errorf("configuration pointer is nil")
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("configuration must be a struct")
	}
	return value, nil
}

func ConfigToMap(config any) (map[string]string, error) {
	value, err := structValue(config, false)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for index := 0; index < value.NumField(); index++ {
		name, included := fieldName(value.Type().Field(index))
		if !included {
			continue
		}
		field := value.Field(index)
		var encoded string
		switch field.Kind() {
		case reflect.String:
			encoded = field.String()
		case reflect.Bool:
			encoded = strconv.FormatBool(field.Bool())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			encoded = strconv.FormatInt(field.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			encoded = strconv.FormatUint(field.Uint(), 10)
		case reflect.Float32, reflect.Float64:
			if math.IsNaN(field.Float()) || math.IsInf(field.Float(), 0) {
				return nil, fmt.Errorf("%s must be finite", name)
			}
			encoded = strconv.FormatFloat(field.Float(), 'f', -1, field.Type().Bits())
		default:
			bytes, err := json.Marshal(field.Interface())
			if err != nil {
				return nil, fmt.Errorf("serialize %s: %w", name, err)
			}
			encoded = string(bytes)
		}
		result[name] = encoded
	}
	return result, nil
}

type configUpdate struct {
	target    reflect.Value
	candidate reflect.Value
	fields    []int
}

func prepareUpdate(config any, values map[string]string) (*configUpdate, error) {
	target, err := structValue(config, true)
	if err != nil {
		return nil, err
	}
	candidate := reflect.New(target.Type()).Elem()
	candidate.Set(target)
	update := &configUpdate{target: target, candidate: candidate}
	for index := 0; index < target.NumField(); index++ {
		name, included := fieldName(target.Type().Field(index))
		if !included {
			continue
		}
		raw, exists := values[name]
		if !exists {
			continue
		}
		fresh, err := parseField(target.Field(index).Type(), raw)
		if err != nil {
			return nil, fmt.Errorf("invalid configuration field %s: %w", name, err)
		}
		candidate.Field(index).Set(fresh)
		update.fields = append(update.fields, index)
	}
	if validator, ok := candidate.Addr().Interface().(interface{ Validate() error }); ok {
		if err := validator.Validate(); err != nil {
			return nil, err
		}
	}
	return update, nil
}

func parseField(fieldType reflect.Type, raw string) (reflect.Value, error) {
	value := reflect.New(fieldType).Elem()
	switch fieldType.Kind() {
	case reflect.String:
		value.SetString(raw)
	case reflect.Bool:
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("expected a boolean")
		}
		value.SetBool(parsed)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		// Legacy options sometimes contain integer values as "2.000000".
		// A rational parse preserves large integers and rejects fractional input.
		parsed, ok := new(big.Rat).SetString(raw)
		if !ok || !parsed.IsInt() {
			return reflect.Value{}, fmt.Errorf("expected an integer")
		}
		integer := parsed.Num()
		if fieldType.Kind() >= reflect.Int && fieldType.Kind() <= reflect.Int64 {
			if !integer.IsInt64() || value.OverflowInt(integer.Int64()) {
				return reflect.Value{}, fmt.Errorf("integer is out of range")
			}
			value.SetInt(integer.Int64())
		} else {
			if !integer.IsUint64() || value.OverflowUint(integer.Uint64()) {
				return reflect.Value{}, fmt.Errorf("unsigned integer is out of range")
			}
			value.SetUint(integer.Uint64())
		}
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(raw, fieldType.Bits())
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return reflect.Value{}, fmt.Errorf("expected a finite number")
		}
		value.SetFloat(parsed)
	default:
		if err := json.Unmarshal([]byte(raw), value.Addr().Interface()); err != nil {
			return reflect.Value{}, fmt.Errorf("expected valid JSON: %w", err)
		}
	}
	return value, nil
}

func (update *configUpdate) commit() error {
	for _, index := range update.fields {
		target := update.target.Field(index)
		candidate := update.candidate.Field(index)
		// RWMap pointers also have package-level aliases. Their JSON decoder
		// swaps internal maps under its own lock, preserving those aliases.
		if target.Kind() == reflect.Pointer && !target.IsNil() && !candidate.IsNil() {
			if decoder, ok := target.Interface().(json.Unmarshaler); ok {
				encoded, err := json.Marshal(candidate.Interface())
				if err != nil {
					return err
				}
				if err := decoder.UnmarshalJSON(encoded); err != nil {
					return err
				}
				continue
			}
		}
		target.Set(candidate)
	}
	return nil
}

func UpdateConfigFromMap(config any, values map[string]string) error {
	update, err := prepareUpdate(config, values)
	if err != nil {
		return err
	}
	return update.commit()
}
