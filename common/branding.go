package common

import "fmt"

func CanonicalBrandingValue(key string) (string, bool) {
	switch key {
	case "SystemName", "passkey.rp_display_name":
		return ProductName, true
	case "Logo":
		return ProductLogo, true
	default:
		return "", false
	}
}

func ValidateBrandingOption(key, value string) error {
	if canonical, fixed := CanonicalBrandingValue(key); fixed && value != canonical {
		return fmt.Errorf("%s is fixed to %s; product branding cannot be changed", key, canonical)
	}
	return nil
}
