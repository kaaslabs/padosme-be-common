package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaaslabs/padosme-be-common/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.env")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func TestLoadEnvFile_BasicKeyValue(t *testing.T) {
	path := writeEnvFile(t, "TEST_FOO=bar\nTEST_NUM=42\n")
	defer os.Unsetenv("TEST_FOO")
	defer os.Unsetenv("TEST_NUM")

	require.NoError(t, config.LoadEnvFile(path))
	assert.Equal(t, "bar", os.Getenv("TEST_FOO"))
	assert.Equal(t, "42", os.Getenv("TEST_NUM"))
}

func TestLoadEnvFile_SkipsComments(t *testing.T) {
	path := writeEnvFile(t, "# this is a comment\nTEST_C=hello\n")
	defer os.Unsetenv("TEST_C")

	require.NoError(t, config.LoadEnvFile(path))
	assert.Equal(t, "hello", os.Getenv("TEST_C"))
}

func TestLoadEnvFile_QuotedValues(t *testing.T) {
	path := writeEnvFile(t, `TEST_DQ="double quoted"` + "\n" + `TEST_SQ='single quoted'` + "\n")
	defer os.Unsetenv("TEST_DQ")
	defer os.Unsetenv("TEST_SQ")

	require.NoError(t, config.LoadEnvFile(path))
	assert.Equal(t, "double quoted", os.Getenv("TEST_DQ"))
	assert.Equal(t, "single quoted", os.Getenv("TEST_SQ"))
}

func TestLoadEnvFile_DoesNotOverwriteExisting(t *testing.T) {
	os.Setenv("TEST_EXISTING", "original")
	defer os.Unsetenv("TEST_EXISTING")

	path := writeEnvFile(t, "TEST_EXISTING=overwritten\n")
	require.NoError(t, config.LoadEnvFile(path))
	assert.Equal(t, "original", os.Getenv("TEST_EXISTING"))
}

func TestLoadEnvFile_MissingFile(t *testing.T) {
	err := config.LoadEnvFile(filepath.Join(t.TempDir(), "nonexistent.env"))
	assert.Error(t, err)
}

func TestGetEnv_ReturnsDefault(t *testing.T) {
	os.Unsetenv("PADOSME_NOT_SET")
	assert.Equal(t, "default", config.GetEnv("PADOSME_NOT_SET", "default"))
}

func TestGetEnv_ReturnsValue(t *testing.T) {
	os.Setenv("PADOSME_SET", "value")
	defer os.Unsetenv("PADOSME_SET")
	assert.Equal(t, "value", config.GetEnv("PADOSME_SET", "default"))
}

func TestMustGetEnv_Panics(t *testing.T) {
	os.Unsetenv("PADOSME_REQUIRED")
	assert.Panics(t, func() { config.MustGetEnv("PADOSME_REQUIRED") })
}

func TestMustGetEnv_Returns(t *testing.T) {
	os.Setenv("PADOSME_REQUIRED", "yes")
	defer os.Unsetenv("PADOSME_REQUIRED")
	assert.Equal(t, "yes", config.MustGetEnv("PADOSME_REQUIRED"))
}
