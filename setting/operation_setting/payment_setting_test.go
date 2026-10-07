package operation_setting

import "testing"

func TestFurtherBillingDoesNotRequireLegacyAcceptance(t *testing.T) {
	original := *GetPaymentSetting()
	t.Cleanup(func() { *GetPaymentSetting() = original })
	GetPaymentSetting().ComplianceConfirmed = false
	GetPaymentSetting().ComplianceTermsVersion = ""
	if !IsPaymentComplianceConfirmed() {
		t.Fatal("Further billing must be available without legacy acceptance")
	}
	if GetPaymentSetting().ComplianceConfirmed || GetPaymentSetting().ComplianceTermsVersion != "" {
		t.Fatal("availability must not fabricate an administrator acceptance record")
	}
}
