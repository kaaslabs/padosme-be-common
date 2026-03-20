package models_test

import (
	"encoding/json"
	"testing"

	"github.com/kaaslabs/padosme-be-common/v2/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntentResult_JSONRoundtrip(t *testing.T) {
	intent := models.IntentResult{
		Intent:          models.IntentProductSearch,
		Category:        models.CategoryRestaurant,
		Product:         "biryani",
		Filters:         []string{"best"},
		Confidence:      0.91,
		Source:          models.SourceOntology,
		TranslatedQuery: "best biryani near me",
		OriginalQuery:   "ಹತ್ತಿರದ ಬಿರಿಯಾನಿ",
		DetectedLang:    "kn",
	}

	b, err := json.Marshal(intent)
	require.NoError(t, err)

	var got models.IntentResult
	require.NoError(t, json.Unmarshal(b, &got))

	assert.Equal(t, intent, got)
}

func TestErrorResponse_JSON(t *testing.T) {
	resp := models.ErrorResponse{
		Error:     "token expired",
		Code:      "UNAUTHORIZED",
		RequestID: "req-123",
	}

	b, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"request_id":"req-123"`)
}

func TestSuccessResponse_JSON(t *testing.T) {
	resp := models.SuccessResponse{
		Data:      map[string]string{"key": "value"},
		RequestID: "req-456",
	}

	b, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"request_id":"req-456"`)
}
