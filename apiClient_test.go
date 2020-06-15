package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetToken(t *testing.T) {
	t.Run("ReturnsToken", func(t *testing.T) {

		if testing.Short() {
			t.Skip("skipping test in short mode.")
		}

		ctx := context.Background()
		getBaseURL := os.Getenv("API_BASE_URL")
		clientID := os.Getenv("CLIENT_ID")
		clientSecret := os.Getenv("CLIENT_SECRET")
		client := NewApiClient(getBaseURL)

		// act
		token, err := client.GetToken(ctx, clientID, clientSecret)

		assert.Nil(t, err)
		assert.True(t, len(token) > 0)
	})
}

func TestGetPipelines(t *testing.T) {
	t.Run("ReturnsPipelines", func(t *testing.T) {

		if testing.Short() {
			t.Skip("skipping test in short mode.")
		}

		ctx := context.Background()
		getBaseURL := os.Getenv("API_BASE_URL")
		clientID := os.Getenv("CLIENT_ID")
		clientSecret := os.Getenv("CLIENT_SECRET")
		client := NewApiClient(getBaseURL)
		token, err := client.GetToken(ctx, clientID, clientSecret)
		assert.Nil(t, err)
		pageNumber := 1
		pageSize := 20
		filters := map[string][]string{
			"search": []string{"estafette-ci"},
			"since":  []string{"eternity"},
		}

		//act
		response, err := client.GetPipelines(ctx, token, pageNumber, pageSize, filters)

		assert.Nil(t, err)
		assert.True(t, response.Pagination.TotalItems > 0)
	})
}
