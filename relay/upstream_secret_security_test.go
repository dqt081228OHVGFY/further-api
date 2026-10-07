package relay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/config"
	"github.com/stretchr/testify/require"
)

func TestTaskSubmitNeverReturnsOpaqueCredentialReflectedByProviderOrPlugin(t *testing.T) {
	service.InitHttpClient()
	const credential = "opaque_plugin_credential_84040"
	const source = `
export const meta = {apiVersion:1,key:"secret-echo",name:"Secret Echo",version:"1.0.0",author:{name:"Test"},models:["declared-model"],fetchMode:"per_task"};
export function buildSubmitRequest(ctx) { return {url:ctx.baseUrl+"/submit",method:"POST",headers:{Authorization:"Bearer "+ctx.authHeader},body:{model:ctx.model},action:"text_to_video"}; }
export function parseSubmitResponse(ctx, response) { throw new Error("provider callback echoed "+ctx.authHeader); }
export function buildQueryRequest(ctx) { return {url:ctx.baseUrl+"/query"}; }
export function parseTaskResult() { return {status:"SUCCESS"}; }
`
	for _, status := range []int{http.StatusUnauthorized, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			saveBillingConfig(t)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_mode": `{"declared-model":"tiered_expr"}`, "billing_setting.billing_expr": `{"declared-model":"tier(\"flat\", 3)"}`}))
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(map[string]any{"message": "rejected " + r.Header.Get("Authorization")})
			}))
			defer upstream.Close()
			c, info := newTaskSubmitContext(t, "declared-model", "")
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
			common.SetContextKey(c, constant.ContextKeyChannelKey, credential)
			c.Set("group", "default")
			info.UserGroup, info.UsingGroup, info.OriginModelName = "default", "default", "declared-model"
			info.Billing = &imageReservation{limit: 1 << 30}
			pinMappingOrderPlugin(t, c, source)
			result, failure := RelayTaskSubmit(c, info)
			require.Nil(t, result)
			require.NotNil(t, failure)
			encoded, err := json.Marshal(failure)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), credential)
			require.NotContains(t, failure.Error.Error(), credential)
			require.Contains(t, failure.Message, "REDACTED")
		})
	}
}
