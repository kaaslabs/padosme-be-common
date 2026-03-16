# pkg/errors

## Purpose
Standard error types and codes shared across all PadosMe backend services.

## Files
| File | Description |
|------|-------------|
| `errors.go` | `AppError` struct, error code constants, predefined constructors |
| `errors_test.go` | Unit tests for all error constructors and methods |

## Usage
```go
import "github.com/kaaslabs/padosme-be-common/pkg/errors"

// Return a structured error
return errors.Unauthorized("token expired", requestID)

// In a Gin handler
appErr := errors.InvalidRequest("missing query field", requestID)
c.JSON(appErr.StatusCode, appErr)
```

## Error Codes
| Constant | Value | HTTP Status |
|----------|-------|-------------|
| `ErrInvalidRequest` | `INVALID_REQUEST` | 400 |
| `ErrUnauthorized` | `UNAUTHORIZED` | 401 |
| `ErrRateLimited` | `RATE_LIMITED` | 429 |
| `ErrIntentFailed` | `INTENT_FAILED` | 502 |
| `ErrSearchFailed` | `SEARCH_FAILED` | 502 |
| `ErrTranslateFailed` | `TRANSLATE_FAILED` | 502 |
| `ErrTimeout` | `TIMEOUT` | 504 |
| `ErrInternalError` | `INTERNAL_ERROR` | 500 |

## Running Tests
```bash
go test -v ./pkg/errors/...
```
