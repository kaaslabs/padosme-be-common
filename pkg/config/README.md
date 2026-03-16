# pkg/config

## Purpose
Environment and .env file configuration loader used by all PadosMe backend services.

## Files
| File | Description |
|------|-------------|
| `loader.go` | `LoadEnvFile`, `GetEnv`, `MustGetEnv` helpers |
| `loader_test.go` | Unit tests for all loader functions |

## Usage
```go
import "github.com/kaaslabs/padosme-be-common/pkg/config"

// At service startup — load .env if it exists (non-fatal if missing)
_ = config.LoadEnvFile(".env")

// Read an optional value with a default
port := config.GetEnv("PADOSME_SEARCH_API_PORT", "8090")

// Read a required value — panics if unset
secret := config.MustGetEnv("PADOSME_JWT_SECRET")
```

## Rules
- `LoadEnvFile` does **not** overwrite existing environment variables (process env takes precedence).
- `MustGetEnv` panics at startup if a required variable is absent — fail fast is intentional.

## Running Tests
```bash
go test -v ./pkg/config/...
```
