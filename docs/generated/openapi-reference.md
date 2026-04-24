# CLIProxyAPI HTTP API Reference

> This file is auto-generated from `openapi.yaml` by `go run ./cmd/generate-openapi-docs`. Do not edit it manually.

## Overview

- **Title:** `CLIProxyAPI HTTP API`
- **Version:** `1.0.0`
- **OpenAPI:** `3.1.0`
- **Tags:** 4
- **Paths:** 27
- **Component Schemas:** 34

OpenAPI schema for the built-in HTTP APIs exposed by CLIProxyAPI. This specification covers the main OpenAI-compatible runtime routes, Gemini-compatible routes, and the most commonly used management endpoints implemented in `internal/api/server.go` and `internal/api/handlers/management`.

## Servers

| URL | Description |
| --- | --- |
| `http://localhost:8080` | Local development server |

## Authentication

Default security requirement: `None`

| Scheme | Type | Description |
| --- | --- | --- |
| `accessBearer` | http / bearer / API key | Runtime API authentication. Most deployments use `Authorization: Bearer <key>`. |
| `managementBearer` | http / bearer / Management key | Management API key sent as `Authorization: Bearer <key>`. |
| `managementKey` | apiKey / header X-Management-Key | Management API key header accepted by `Handler.Middleware`. |

## Health Endpoints

| Method | Path | Summary | Request Body | Responses |
| --- | --- | --- | --- | --- |
| GET | `/` | Describe the runtime API entrypoint | None | 200 → application/json → RootInfo |
| GET | `/healthz` | Health check | None | 200 → application/json → HealthStatus |

### GET `/`

Describe the runtime API entrypoint

- **Operation ID:** `getRootInfo`
- **Security:** `None`
- **Tags:** `Health`

#### Parameters

None.

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Root endpoint summary | application/json → RootInfo |

### GET `/healthz`

Health check

- **Operation ID:** `getHealth`
- **Security:** `None`
- **Tags:** `Health`

#### Parameters

None.

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Server is healthy | application/json → HealthStatus |

## Runtime Endpoints

| Method | Path | Summary | Request Body | Responses |
| --- | --- | --- | --- | --- |
| POST | `/v1/chat/completions` | Run an OpenAI-compatible chat completion | Required: application/json → ChatCompletionsRequest | 200 → application/json → ChatCompletionsResponse; default → application/json → ErrorResponse |
| POST | `/v1/completions` | Run an OpenAI-compatible text completion | Required: application/json → CompletionRequest | 200 → application/json → CompletionResponse; default → application/json → ErrorResponse |
| POST | `/v1/messages` | Run a Claude-compatible message request | Required: application/json → ClaudeMessagesRequest | 200 → application/json → ClaudeMessageResponse; default → application/json → ErrorResponse |
| POST | `/v1/messages/count_tokens` | Count Claude message tokens | Required: application/json → ClaudeMessagesRequest | 200 → application/json → CountTokensResponse; default → application/json → ErrorResponse |
| GET | `/v1/models` | List available runtime models | None | 200 → application/json → OpenAIModelList \| ClaudeModelList; default → application/json → ErrorResponse |
| GET | `/v1/responses` | OpenAI Responses websocket endpoint | None | 101 → —; default → application/json → ErrorResponse |
| POST | `/v1/responses` | Run an OpenAI Responses request | Required: application/json → OpenAIResponsesRequest | 200 → application/json → OpenAIResponsesResponse; default → application/json → ErrorResponse |
| POST | `/v1/responses/compact` | Run a compact non-streaming OpenAI Responses request | Required: application/json → OpenAIResponsesRequest | 200 → application/json → OpenAIResponsesResponse; default → application/json → ErrorResponse |

### POST `/v1/chat/completions`

Run an OpenAI-compatible chat completion

When `stream=true`, the server upgrades the response to Server-Sent Events.

- **Operation ID:** `createChatCompletion`
- **Security:** `accessBearer`
- **Tags:** `Runtime`

#### Parameters

None.

#### Request Body

- Required: Yes

| Content Type | Schema |
| --- | --- |
| `application/json` | ChatCompletionsRequest |

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Completion response | application/json → ChatCompletionsResponse |
| `default` | Error response | application/json → ErrorResponse |

### POST `/v1/completions`

Run an OpenAI-compatible text completion

This route is internally converted to chat completions.

- **Operation ID:** `createCompletion`
- **Security:** `accessBearer`
- **Tags:** `Runtime`

#### Parameters

None.

#### Request Body

- Required: Yes

| Content Type | Schema |
| --- | --- |
| `application/json` | CompletionRequest |

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Completion response | application/json → CompletionResponse |
| `default` | Error response | application/json → ErrorResponse |

### POST `/v1/messages`

Run a Claude-compatible message request

Returns SSE when `stream=true`, otherwise returns the translated upstream JSON payload.

- **Operation ID:** `createClaudeMessage`
- **Security:** `accessBearer`
- **Tags:** `Runtime`

#### Parameters

None.

#### Request Body

- Required: Yes

| Content Type | Schema |
| --- | --- |
| `application/json` | ClaudeMessagesRequest |

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Claude message response | application/json → ClaudeMessageResponse |
| `default` | Error response | application/json → ErrorResponse |

### POST `/v1/messages/count_tokens`

Count Claude message tokens

- **Operation ID:** `countClaudeTokens`
- **Security:** `accessBearer`
- **Tags:** `Runtime`

#### Parameters

None.

#### Request Body

- Required: Yes

| Content Type | Schema |
| --- | --- |
| `application/json` | ClaudeMessagesRequest |

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Token count payload | application/json → CountTokensResponse |
| `default` | Error response | application/json → ErrorResponse |

### GET `/v1/models`

List available runtime models

Returns an OpenAI-compatible list by default. Claude CLI callers receive a Claude-style list shape.

- **Operation ID:** `listRuntimeModels`
- **Security:** `accessBearer`
- **Tags:** `Runtime`

#### Parameters

None.

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Available models | application/json → OpenAIModelList \| ClaudeModelList |
| `default` | Error response | application/json → ErrorResponse |

### GET `/v1/responses`

OpenAI Responses websocket endpoint

Upgrades to a websocket connection for the Responses API transport.

- **Operation ID:** `openResponsesWebsocket`
- **Security:** `accessBearer`
- **Tags:** `Runtime`

#### Parameters

None.

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `101` | Switching protocols | — |
| `default` | Error response | application/json → ErrorResponse |

### POST `/v1/responses`

Run an OpenAI Responses request

Returns JSON for non-streaming requests and SSE when `stream=true`.

- **Operation ID:** `createResponse`
- **Security:** `accessBearer`
- **Tags:** `Runtime`

#### Parameters

None.

#### Request Body

- Required: Yes

| Content Type | Schema |
| --- | --- |
| `application/json` | OpenAIResponsesRequest |

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Responses API result | application/json → OpenAIResponsesResponse |
| `default` | Error response | application/json → ErrorResponse |

### POST `/v1/responses/compact`

Run a compact non-streaming OpenAI Responses request

Rejects `stream=true` and returns a compact JSON payload.

- **Operation ID:** `createCompactResponse`
- **Security:** `accessBearer`
- **Tags:** `Runtime`

#### Parameters

None.

#### Request Body

- Required: Yes

| Content Type | Schema |
| --- | --- |
| `application/json` | OpenAIResponsesRequest |

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Compact responses result | application/json → OpenAIResponsesResponse |
| `default` | Error response | application/json → ErrorResponse |

## Gemini Endpoints

| Method | Path | Summary | Request Body | Responses |
| --- | --- | --- | --- | --- |
| GET | `/v1beta/models` | List Gemini-compatible models | None | 200 → application/json → GeminiModelsResponse; default → application/json → ErrorResponse |
| GET | `/v1beta/models/{action}` | Get Gemini model metadata | None | 200 → application/json → ModelSummary; default → application/json → ErrorResponse |
| POST | `/v1beta/models/{action}` | Invoke a Gemini model method | Required: application/json → GeminiGenerateContentRequest | 200 → application/json → GeminiGenerateContentResponse \| CountTokensResponse; default → application/json → ErrorResponse |

### GET `/v1beta/models`

List Gemini-compatible models

- **Operation ID:** `listGeminiModels`
- **Security:** `accessBearer`
- **Tags:** `Gemini`

#### Parameters

None.

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Gemini model list | application/json → GeminiModelsResponse |
| `default` | Error response | application/json → ErrorResponse |

### GET `/v1beta/models/{action}`

Get Gemini model metadata

- **Operation ID:** `getGeminiModel`
- **Security:** `accessBearer`
- **Tags:** `Gemini`

#### Parameters

None.

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Gemini model metadata | application/json → ModelSummary |
| `default` | Error response | application/json → ErrorResponse |

### POST `/v1beta/models/{action}`

Invoke a Gemini model method

Supported methods are `generateContent`, `streamGenerateContent`, and `countTokens`.

- **Operation ID:** `invokeGeminiModelMethod`
- **Security:** `accessBearer`
- **Tags:** `Gemini`

#### Parameters

None.

#### Request Body

- Required: Yes

| Content Type | Schema |
| --- | --- |
| `application/json` | GeminiGenerateContentRequest |

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Gemini model result | application/json → GeminiGenerateContentResponse \| CountTokensResponse |
| `default` | Error response | application/json → ErrorResponse |

## Management Endpoints

| Method | Path | Summary | Request Body | Responses |
| --- | --- | --- | --- | --- |
| GET | `/v0/management/anthropic-auth-url` | Start a Claude OAuth flow | None | 200 → application/json → OAuthBootstrapResponse |
| GET | `/v0/management/antigravity-auth-url` | Start an Antigravity OAuth flow | None | 200 → application/json → OAuthBootstrapResponse |
| GET | `/v0/management/auth-files` | List auth files | None | 200 → application/json → AuthFileListResponse |
| POST | `/v0/management/auth-files` | Upload one or more auth files | Required: multipart/form-data → object | 200 → application/json → StatusResponse; 207 → application/json → StatusResponse |
| DELETE | `/v0/management/auth-files` | Delete one or more auth files | None | 200 → application/json → StatusResponse; 207 → application/json → StatusResponse |
| PATCH | `/v0/management/auth-files/fields` | Update editable auth file fields | Required: application/json → AuthFileFieldsPatchRequest | 200 → application/json → StatusResponse |
| GET | `/v0/management/auth-files/models` | List models reachable through a specific auth file | None | 200 → application/json → AuthFileModelsResponse |
| PATCH | `/v0/management/auth-files/status` | Enable or disable an auth file | Required: application/json → AuthFileStatusPatchRequest | 200 → application/json → StatusResponse |
| GET | `/v0/management/codex-auth-url` | Start a Codex OAuth flow | None | 200 → application/json → OAuthBootstrapResponse |
| GET | `/v0/management/config` | Get the in-memory server config | None | 200 → application/json → ConfigDocument |
| GET | `/v0/management/config.yaml` | Download the raw config YAML | None | 200 → application/yaml → string |
| PUT | `/v0/management/config.yaml` | Replace the config YAML | Required: application/yaml → string | 200 → application/json → ConfigMutationResponse; 422 → application/json → object |
| GET | `/v0/management/gemini-cli-auth-url` | Start a Gemini CLI OAuth flow | None | 200 → application/json → OAuthBootstrapResponse |
| GET | `/v0/management/get-auth-status` | Query the status of a pending OAuth flow | None | 200 → application/json → AuthStatusResponse; 400 → application/json → AuthStatusResponse |
| GET | `/v0/management/kimi-auth-url` | Start a Kimi OAuth flow | None | 200 → application/json → OAuthBootstrapResponse |
| GET | `/v0/management/model-definitions/{channel}` | Get static model definitions for a channel | None | 200 → application/json → ModelDefinitionsResponse |
| POST | `/v0/management/oauth-callback` | Persist an OAuth callback for a pending session | Required: application/json → OAuthCallbackRequest | 200 → application/json → StatusResponse; 400 → application/json → StatusResponse; 404 → application/json → StatusResponse; 409 → application/json → StatusResponse |
| GET | `/v0/management/usage` | Read in-memory request statistics | None | 200 → application/json → UsageStatisticsResponse |
| POST | `/v0/management/vertex/import` | Import a Vertex service account credential | Required: multipart/form-data → object | 200 → application/json → VertexImportResponse |

### GET `/v0/management/anthropic-auth-url`

Start a Claude OAuth flow

- **Operation ID:** `getAnthropicAuthUrl`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

None.

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | OAuth bootstrap URL | application/json → OAuthBootstrapResponse |

### GET `/v0/management/antigravity-auth-url`

Start an Antigravity OAuth flow

- **Operation ID:** `getAntigravityAuthUrl`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

None.

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | OAuth bootstrap URL | application/json → OAuthBootstrapResponse |

### GET `/v0/management/auth-files`

List auth files

- **Operation ID:** `listAuthFiles`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

None.

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Auth files registered in the auth manager or on disk | application/json → AuthFileListResponse |

### POST `/v0/management/auth-files`

Upload one or more auth files

Multipart upload is the primary flow. The handler also supports raw JSON bodies when `?name=<file>.json` is provided.

- **Operation ID:** `uploadAuthFile`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

None.

#### Request Body

- Required: Yes

| Content Type | Schema |
| --- | --- |
| `multipart/form-data` | object |

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Upload result | application/json → StatusResponse |
| `207` | Partial upload result | application/json → StatusResponse |

### DELETE `/v0/management/auth-files`

Delete one or more auth files

- **Operation ID:** `deleteAuthFile`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

| Name | In | Required | Schema | Description |
| --- | --- | --- | --- | --- |
| `all` | `query` | No | boolean | — |
| `name` | `query` | No | array<string> | One or more auth file names to delete. |

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Delete result | application/json → StatusResponse |
| `207` | Partial delete result | application/json → StatusResponse |

### PATCH `/v0/management/auth-files/fields`

Update editable auth file fields

- **Operation ID:** `patchAuthFileFields`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

None.

#### Request Body

- Required: Yes

| Content Type | Schema |
| --- | --- |
| `application/json` | AuthFileFieldsPatchRequest |

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Updated auth file metadata | application/json → StatusResponse |

### GET `/v0/management/auth-files/models`

List models reachable through a specific auth file

- **Operation ID:** `getAuthFileModels`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

| Name | In | Required | Schema | Description |
| --- | --- | --- | --- | --- |
| `name` | `query` | Yes | string | — |

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Models for the selected auth file | application/json → AuthFileModelsResponse |

### PATCH `/v0/management/auth-files/status`

Enable or disable an auth file

- **Operation ID:** `patchAuthFileStatus`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

None.

#### Request Body

- Required: Yes

| Content Type | Schema |
| --- | --- |
| `application/json` | AuthFileStatusPatchRequest |

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Updated auth file status | application/json → StatusResponse |

### GET `/v0/management/codex-auth-url`

Start a Codex OAuth flow

- **Operation ID:** `getCodexAuthUrl`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

None.

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | OAuth bootstrap URL | application/json → OAuthBootstrapResponse |

### GET `/v0/management/config`

Get the in-memory server config

- **Operation ID:** `getManagementConfig`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

None.

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Current config snapshot | application/json → ConfigDocument |

### GET `/v0/management/config.yaml`

Download the raw config YAML

- **Operation ID:** `getManagementConfigYaml`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

None.

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Raw YAML content | application/yaml → string |

### PUT `/v0/management/config.yaml`

Replace the config YAML

- **Operation ID:** `putManagementConfigYaml`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

None.

#### Request Body

- Required: Yes

| Content Type | Schema |
| --- | --- |
| `application/yaml` | string |

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Config written and reloaded | application/json → ConfigMutationResponse |
| `422` | Invalid config payload | application/json → object |

### GET `/v0/management/gemini-cli-auth-url`

Start a Gemini CLI OAuth flow

- **Operation ID:** `getGeminiCliAuthUrl`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

| Name | In | Required | Schema | Description |
| --- | --- | --- | --- | --- |
| `project_id` | `query` | No | string | Optional Gemini project override. |

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | OAuth bootstrap URL | application/json → OAuthBootstrapResponse |

### GET `/v0/management/get-auth-status`

Query the status of a pending OAuth flow

- **Operation ID:** `getAuthStatus`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

| Name | In | Required | Schema | Description |
| --- | --- | --- | --- | --- |
| `state` | `query` | No | string | — |

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Current OAuth state | application/json → AuthStatusResponse |
| `400` | Invalid state value | application/json → AuthStatusResponse |

### GET `/v0/management/kimi-auth-url`

Start a Kimi OAuth flow

- **Operation ID:** `getKimiAuthUrl`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

None.

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | OAuth bootstrap URL | application/json → OAuthBootstrapResponse |

### GET `/v0/management/model-definitions/{channel}`

Get static model definitions for a channel

- **Operation ID:** `getModelDefinitions`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

| Name | In | Required | Schema | Description |
| --- | --- | --- | --- | --- |
| `channel` | `path` | Yes | string | — |

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Static model definitions for the channel | application/json → ModelDefinitionsResponse |

### POST `/v0/management/oauth-callback`

Persist an OAuth callback for a pending session

- **Operation ID:** `postOAuthCallback`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

None.

#### Request Body

- Required: Yes

| Content Type | Schema |
| --- | --- |
| `application/json` | OAuthCallbackRequest |

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Callback persisted | application/json → StatusResponse |
| `400` | Invalid callback payload | application/json → StatusResponse |
| `404` | Unknown or expired state | application/json → StatusResponse |
| `409` | OAuth flow is not pending | application/json → StatusResponse |

### GET `/v0/management/usage`

Read in-memory request statistics

- **Operation ID:** `getUsageStatistics`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

None.

#### Request Body

None.

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Usage statistics snapshot | application/json → UsageStatisticsResponse |

### POST `/v0/management/vertex/import`

Import a Vertex service account credential

- **Operation ID:** `importVertexCredential`
- **Security:** `managementKey or managementBearer`
- **Tags:** `Management`

#### Parameters

None.

#### Request Body

- Required: Yes

| Content Type | Schema |
| --- | --- |
| `multipart/form-data` | object |

#### Responses

| Status | Description | Schema |
| --- | --- | --- |
| `200` | Imported Vertex credential | application/json → VertexImportResponse |

## Component Schemas

| Schema | Type | Properties | Required | Description |
| --- | --- | ---: | ---: | --- |
| `AuthFileFieldsPatchRequest` | object | 6 | 1 | — |
| `AuthFileListResponse` | object | 1 | 0 | — |
| `AuthFileModelsResponse` | object | 1 | 0 | — |
| `AuthFileStatusPatchRequest` | object | 2 | 2 | — |
| `AuthFileSummary` | object | 9 | 0 | — |
| `AuthStatusResponse` | object | 2 | 0 | — |
| `ChatCompletionsRequest` | object | 7 | 1 | — |
| `ChatCompletionsResponse` | object | 6 | 0 | — |
| `ChatMessage` | object | 2 | 2 | — |
| `ClaudeMessageResponse` | object | 5 | 0 | — |
| `ClaudeMessagesRequest` | object | 5 | 2 | — |
| `ClaudeModelList` | object | 4 | 2 | — |
| `CompletionRequest` | object | 5 | 2 | — |
| `CompletionResponse` | object | 6 | 0 | — |
| `ConfigDocument` | object | 0 | 0 | — |
| `ConfigMutationResponse` | object | 2 | 0 | — |
| `CountTokensResponse` | object | 2 | 0 | — |
| `ErrorDetail` | object | 3 | 2 | — |
| `ErrorResponse` | object | 1 | 1 | — |
| `GeminiGenerateContentRequest` | object | 3 | 0 | — |
| `GeminiGenerateContentResponse` | object | 3 | 0 | — |
| `GeminiModelsResponse` | object | 1 | 1 | — |
| `HealthStatus` | object | 1 | 1 | — |
| `ModelDefinitionsResponse` | object | 2 | 0 | — |
| `ModelSummary` | object | 8 | 0 | — |
| `OAuthBootstrapResponse` | object | 3 | 0 | — |
| `OAuthCallbackRequest` | object | 5 | 1 | — |
| `OpenAIModelList` | object | 2 | 2 | — |
| `OpenAIResponsesRequest` | object | 4 | 1 | — |
| `OpenAIResponsesResponse` | object | 5 | 0 | — |
| `RootInfo` | object | 2 | 2 | — |
| `StatusResponse` | object | 5 | 0 | — |
| `UsageStatisticsResponse` | object | 2 | 0 | — |
| `VertexImportResponse` | object | 5 | 0 | — |
