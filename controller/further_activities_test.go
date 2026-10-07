package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFurtherActivityPreferenceAcceptsNewPageAndMigratesAliases(t *testing.T) {
	db := setupFurtherRegistrationController(t)
	require.NoError(t, db.AutoMigrate(&model.FurtherUIPreferences{}))
	engine := gin.New()
	engine.PATCH("/prefs", func(c *gin.Context) { c.Set("id", 7); c.Next() }, FurtherPatchPreferences)
	for _, page := range []string{"activities", "benefits", "checkin", "activity"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPatch, "/prefs", strings.NewReader(`{"default_page":"`+page+`"}`))
		request.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Success bool `json:"success"`
			Data    struct {
				DefaultPage string `json:"default_page"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.True(t, response.Success)
		require.Equal(t, "activities", response.Data.DefaultPage)
		var stored model.FurtherUIPreferences
		require.NoError(t, db.First(&stored, "user_id = ?", 7).Error)
		require.Equal(t, "activities", stored.DefaultPage)
	}
}
