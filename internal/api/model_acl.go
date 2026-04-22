package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/tidwall/gjson"
)

// modelACLMaxBodyBytes caps the request body the ACL middleware will inspect in
// order to extract the model field. Oversized bodies fail closed with HTTP 413.
const modelACLMaxBodyBytes int64 = 10 * 1024 * 1024 // 10 MiB

// modelACLPeekBytes is the initial body window inspected before buffering the
// remainder. Most request shapes place "model" near the top of the payload.
const modelACLPeekBytes int64 = 16 * 1024 // 16 KiB

var errBodyTooLarge = errors.New("model_acl: request body exceeds cap")

const imageRouteDefaultModel = "gpt-image-2"
const imageRouteExecutionModel = "gpt-5.4-mini"
const imageRouteMultipartMaxBytes int64 = 32 << 20

// ModelACLMiddleware enforces per-key model allowlists for the routes it is
// installed on. The config closure is resolved on every request so hot reloads
// take effect immediately.
func ModelACLMiddleware(cfgFn func() *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := cfgFn()
		if cfg == nil {
			c.Next()
			return
		}

		if !cfg.ModelACLConfigured() {
			c.Next()
			return
		}

		raw, exists := c.Get("apiKey")
		if !exists {
			c.Next()
			return
		}

		apiKey, ok := raw.(string)
		if !ok || strings.TrimSpace(apiKey) == "" {
			c.Next()
			return
		}

		if isWebsocketUpgradeRequest(c.Request) && cfg.IsAPIKeyModelRestricted(apiKey) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"type":    "websocket_not_allowed_for_restricted_key",
					"message": "model-restricted api keys cannot use websocket upgrade routes; model selection happens after the upgrade",
				},
			})
			return
		}

		if !cfg.IsAPIKeyModelRestricted(apiKey) {
			c.Next()
			return
		}

		model, found, err := extractRequestedModel(c)
		if err != nil {
			if errors.Is(err, errBodyTooLarge) {
				c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
					"error": gin.H{
						"type":    "request_too_large",
						"message": "request body exceeds the model ACL inspection cap",
					},
				})
				return
			}

			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"type":    "invalid_request_body",
					"message": "could not read request body for model ACL enforcement",
				},
			})
			return
		}

		if !found {
			c.Next()
			return
		}

		if cfg.IsModelAllowedForKey(apiKey, model) {
			if isImageRoute(c.Request.URL.Path) && !cfg.IsModelAllowedForKey(apiKey, imageRouteExecutionModel) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": gin.H{
						"type":            "model_not_allowed_for_key",
						"message":         "this api key is not permitted to use the backing model required for image routes",
						"model":           imageRouteExecutionModel,
						"requested_model": model,
					},
				})
				return
			}
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"type":    "model_not_allowed_for_key",
				"message": "this api key is not permitted to use the requested model",
				"model":   model,
			},
		})
	}
}

// extractRequestedModel returns the model identifier targeted by the request.
// It returns found=false for routes that do not carry a model identifier.
func extractRequestedModel(c *gin.Context) (model string, found bool, err error) {
	if c == nil || c.Request == nil {
		return "", false, nil
	}

	if model, found, err = extractImageRouteModel(c); found || err != nil {
		return model, found, err
	}

	if idx := strings.Index(c.Request.URL.Path, "/v1beta/models/"); idx >= 0 {
		rest := c.Request.URL.Path[idx+len("/v1beta/models/"):]
		if idx := strings.Index(rest, ":"); idx >= 0 {
			rest = rest[:idx]
		}
		rest = strings.TrimSpace(rest)
		if rest != "" {
			return rest, true, nil
		}
	}

	if strings.Contains(c.Request.URL.Path, "/v1beta1/") {
		if idx := strings.Index(c.Request.URL.Path, "/models/"); idx >= 0 {
			rest := c.Request.URL.Path[idx+len("/models/"):]
			if idx := strings.Index(rest, ":"); idx >= 0 {
				rest = rest[:idx]
			}
			rest = strings.TrimSpace(rest)
			if rest != "" {
				return rest, true, nil
			}
		}
	}

	switch c.Request.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return "", false, nil
	}

	if c.Request.Body == nil {
		return "", false, nil
	}

	if c.Request.ContentLength > modelACLMaxBodyBytes {
		return "", false, errBodyTooLarge
	}

	peek := make([]byte, modelACLPeekBytes)
	peekN, peekErr := io.ReadFull(c.Request.Body, peek)
	peek = peek[:peekN]
	bodyFullyRead := peekErr == io.EOF || peekErr == io.ErrUnexpectedEOF
	if peekErr != nil && !bodyFullyRead {
		return "", false, peekErr
	}

	if bodyFullyRead {
		c.Request.Body = io.NopCloser(bytes.NewReader(peek))
		model, found, err = extractModelFromBytes(peek)
		if err != nil {
			return "", false, err
		}
		if found {
			return model, true, nil
		}
		if isImageRoute(c.Request.URL.Path) {
			return imageRouteDefaultModel, true, nil
		}
		return "", false, nil
	}

	if model, found, _ := extractModelFromBytes(peek); found {
		c.Request.Body = io.NopCloser(io.MultiReader(bytes.NewReader(peek), c.Request.Body))
		return model, true, nil
	}

	remaining := modelACLMaxBodyBytes - int64(len(peek))
	if remaining <= 0 {
		return "", false, errBodyTooLarge
	}

	limited := io.LimitReader(c.Request.Body, remaining+1)
	rest, readErr := io.ReadAll(limited)
	if readErr != nil {
		return "", false, readErr
	}
	if int64(len(rest)) > remaining {
		return "", false, errBodyTooLarge
	}

	bodyBytes := make([]byte, 0, len(peek)+len(rest))
	bodyBytes = append(bodyBytes, peek...)
	bodyBytes = append(bodyBytes, rest...)
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	model, found, err = extractModelFromBytes(bodyBytes)
	if err != nil {
		return "", false, err
	}
	if found {
		return model, true, nil
	}
	if isImageRoute(c.Request.URL.Path) {
		return imageRouteDefaultModel, true, nil
	}
	return "", false, nil
}

func extractImageRouteModel(c *gin.Context) (model string, found bool, err error) {
	if c == nil || c.Request == nil || !isImageRoute(c.Request.URL.Path) {
		return "", false, nil
	}

	contentType := strings.ToLower(strings.TrimSpace(c.Request.Header.Get("Content-Type")))
	if strings.HasSuffix(c.Request.URL.Path, "/images/edits") && strings.HasPrefix(contentType, "multipart/form-data") {
		if c.Request.ContentLength > imageRouteMultipartMaxBytes {
			return "", false, errBodyTooLarge
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, imageRouteMultipartMaxBytes)
		}
		if err := c.Request.ParseMultipartForm(imageRouteMultipartMaxBytes); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				return "", false, errBodyTooLarge
			}
			return "", false, err
		}
		model = strings.TrimSpace(c.Request.FormValue("model"))
		if model == "" {
			model = imageRouteDefaultModel
		}
		return model, true, nil
	}

	return "", false, nil
}

func isImageRoute(path string) bool {
	path = strings.TrimSpace(path)
	return strings.HasSuffix(path, "/v1/images/generations") || strings.HasSuffix(path, "/v1/images/edits")
}

func extractModelFromBytes(body []byte) (model string, found bool, err error) {
	if len(body) == 0 {
		return "", false, nil
	}

	res := gjson.GetBytes(body, "model")
	if !res.Exists() || res.Type != gjson.String {
		return "", false, nil
	}

	model = strings.TrimSpace(res.String())
	if model == "" {
		return "", false, nil
	}

	return model, true, nil
}

func isWebsocketUpgradeRequest(req *http.Request) bool {
	if req == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(req.Header.Get("Upgrade")), "websocket")
}
