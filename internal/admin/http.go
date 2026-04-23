package admin

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func NewHandler(service *Service) http.Handler {
	engine := gin.New()
	engine.HandleMethodNotAllowed = true
	engine.Use(gin.Recovery(), corsMiddleware())

	adminRoutes := engine.Group("/api/admin")
	adminRoutes.GET("/settings", func(context *gin.Context) {
		context.JSON(http.StatusOK, service.Current())
	})
	adminRoutes.PUT("/settings", func(context *gin.Context) {
		payload, ok := decodeUpdateSettingsRequest(context, false)
		if !ok {
			return
		}

		settings, err := service.Update(context.Request.Context(), payload)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}

		context.JSON(http.StatusOK, settings)
	})
	adminRoutes.POST("/settings/test", func(context *gin.Context) {
		payload, ok := decodeUpdateSettingsRequest(context, true)
		if !ok {
			return
		}

		result, err := service.Test(context.Request.Context(), payload)
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}

		context.JSON(http.StatusOK, result)
	})
	adminRoutes.GET("/users", func(context *gin.Context) {
		context.JSON(http.StatusOK, gin.H{
			"items": []any{},
		})
	})
	adminRoutes.GET("/health", func(context *gin.Context) {
		context.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	return engine
}

func corsMiddleware() gin.HandlerFunc {
	return func(context *gin.Context) {
		context.Header("Access-Control-Allow-Origin", "*")
		context.Header("Access-Control-Allow-Headers", "Content-Type")
		context.Header("Access-Control-Allow-Methods", "GET, PUT, POST, OPTIONS")

		if context.Request.Method == http.MethodOptions {
			context.Status(http.StatusNoContent)
			context.Abort()
			return
		}

		context.Next()
	}
}

func decodeUpdateSettingsRequest(context *gin.Context, allowEmptyBody bool) (UpdateSettingsRequest, bool) {
	var payload UpdateSettingsRequest
	if context.Request.Body == nil {
		return payload, true
	}

	if err := json.NewDecoder(context.Request.Body).Decode(&payload); err != nil {
		if allowEmptyBody && errors.Is(err, http.ErrBodyNotAllowed) {
			return payload, true
		}
		if allowEmptyBody && err.Error() == "EOF" {
			return payload, true
		}

		context.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_json",
		})
		return UpdateSettingsRequest{}, false
	}

	return payload, true
}
