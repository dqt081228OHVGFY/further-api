package business

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/stretchr/testify/require"
)

func TestWeChatAccessUsesEncodedCodeAndConfiguredCredential(t *testing.T) {
	oldEnabled, oldAddress, oldToken := common.WeChatAuthEnabled, common.WeChatServerAddress, common.WeChatServerToken
	t.Cleanup(func() {
		common.WeChatAuthEnabled, common.WeChatServerAddress, common.WeChatServerToken = oldEnabled, oldAddress, oldToken
	})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/connector/api/wechat/user", r.URL.Path)
		require.Equal(t, "code +&=", r.URL.Query().Get("code"))
		require.Equal(t, "fixture-credential", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"success":true,"data":"verified-subject"}`))
	}))
	defer upstream.Close()
	common.WeChatAuthEnabled, common.WeChatServerAddress, common.WeChatServerToken = true, upstream.URL+"/connector", "fixture-credential"
	assertion, err := ExchangeWeChatCodeContext(context.Background(), "code +&=")
	require.NoError(t, err)
	require.Equal(t, "verified-subject", assertion.Subject)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ExchangeWeChatCodeContext(ctx, "code")
	require.ErrorIs(t, err, context.Canceled)
}
