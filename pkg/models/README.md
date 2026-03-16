# pkg/models

## Purpose
Shared data models used across all PadosMe backend services.

## Files
| File | Description |
|------|-------------|
| `intent.go` | `IntentResult` struct and its constants (intent types, sources, categories) |
| `response.go` | `ErrorResponse` and `SuccessResponse` standard API response wrappers |
| `models_test.go` | JSON round-trip and serialisation tests |

## Usage
```go
import "github.com/kaaslabs/padosme-be-common/pkg/models"

// Standard error response in a Gin handler
c.JSON(http.StatusUnauthorized, models.ErrorResponse{
    Error:     "token expired",
    Code:      "UNAUTHORIZED",
    RequestID: requestID,
})

// Intent result from padosme-query-intelligence
intent := models.IntentResult{
    Intent:   models.IntentProductSearch,
    Category: models.CategoryRestaurant,
    Product:  "biryani",
    Source:   models.SourceOntology,
}
```

## Running Tests
```bash
go test -v ./pkg/models/...
```
