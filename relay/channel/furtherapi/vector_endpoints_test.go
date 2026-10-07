package furtherapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayVectorEndpoints(t *testing.T) {
	dimensions, topN := 2, 1
	for _, tc := range []struct {
		name, path, response string
		mode                 int
		format               types.RelayFormat
		request              any
		convert              func(*Adaptor, *relaycommon.RelayInfo) (any, error)
	}{
		{
			name: "embeddings", path: "/v1/embeddings", mode: relayconstant.RelayModeEmbeddings, format: types.RelayFormatEmbedding,
			request:  dto.EmbeddingRequest{Model: "embedding-model", Input: []string{"hello"}, Dimensions: &dimensions, EncodingFormat: "float"},
			response: `{"object":"list","model":"embedding-model","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],"usage":{"prompt_tokens":3,"total_tokens":3}}`,
			convert: func(a *Adaptor, info *relaycommon.RelayInfo) (any, error) {
				return a.ConvertEmbeddingRequest(nil, info, dto.EmbeddingRequest{Model: "embedding-model", Input: []string{"hello"}, Dimensions: &dimensions, EncodingFormat: "float"})
			},
		},
		{
			name: "rerank", path: "/v1/rerank", mode: relayconstant.RelayModeRerank, format: types.RelayFormatRerank,
			request:  dto.RerankRequest{Model: "rerank-model", Query: "hello", Documents: []any{"first", "second"}, TopN: &topN},
			response: `{"results":[{"index":1,"relevance_score":0.9}],"usage":{"total_tokens":3}}`,
			convert: func(a *Adaptor, info *relaycommon.RelayInfo) (any, error) {
				return a.ConvertRerankRequest(nil, info.RelayMode, dto.RerankRequest{Model: "rerank-model", Query: "hello", Documents: []any{"first", "second"}, TopN: &topN})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requestJSON, err := json.Marshal(tc.request)
			require.NoError(t, err)
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/prefix"+tc.path, r.URL.Path)
				assert.Equal(t, "trace=1", r.URL.RawQuery)
				assert.Equal(t, "Bearer gateway-secret", r.Header.Get("Authorization"))
				body, readErr := io.ReadAll(r.Body)
				assert.NoError(t, readErr)
				assert.JSONEq(t, string(requestJSON), string(body))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.response)
			}))
			defer gateway.Close()
			info := &relaycommon.RelayInfo{
				ChannelMeta:    &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeGateway, ChannelBaseUrl: gateway.URL + "/prefix/", ApiKey: "gateway-secret"},
				RequestURLPath: tc.path + "?trace=1", RelayMode: tc.mode, RelayFormat: tc.format,
			}
			adaptor := &Adaptor{}
			adaptor.Init(info)
			converted, err := tc.convert(adaptor, info)
			require.NoError(t, err)
			body, err := json.Marshal(converted)
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, tc.path, nil)
			ctx.Request.Header.Set("Content-Type", "application/json")
			response, err := adaptor.DoRequest(ctx, info, bytes.NewReader(body))
			require.NoError(t, err)
			usage, apiErr := adaptor.DoResponse(ctx, response.(*http.Response), info)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 3, usage.(*dto.Usage).TotalTokens)
			assert.Equal(t, http.StatusOK, recorder.Code)
			var expected, received map[string]any
			require.NoError(t, json.Unmarshal([]byte(tc.response), &expected))
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &received))
			if tc.mode == relayconstant.RelayModeEmbeddings {
				assert.Equal(t, expected["data"], received["data"])
			} else {
				assert.Equal(t, expected["results"], received["results"])
			}
		})
	}
}
