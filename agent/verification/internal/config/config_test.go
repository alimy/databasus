package config

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func emptyReader() *bufio.Reader {
	return bufio.NewReader(strings.NewReader(""))
}

func Test_Validate_WhenConfigComplete_ReturnsCapacity(t *testing.T) {
	cfg := &Config{
		DatabasusHost:     "https://primary.example:4005",
		AgentID:           "agent-1",
		Token:             "secret",
		MaxCPU:            8,
		MaxRAMMb:          4096,
		MaxDiskGb:         100,
		MaxConcurrentJobs: 4,
	}

	capacity, err := cfg.Validate()

	require.NoError(t, err)
	assert.Equal(t, 2, capacity.CPUPerJob)
	assert.Equal(t, 1024, capacity.RAMMbPerJob)
}

func Test_Validate_WhenRequiredStringMissing_ReturnsError(t *testing.T) {
	for name, cfg := range map[string]*Config{
		"no databasus-host": {AgentID: "a", Token: "t", MaxCPU: 4, MaxRAMMb: 2048, MaxDiskGb: 10, MaxConcurrentJobs: 1},
		"no agent-id":       {DatabasusHost: "https://x:4005", Token: "t", MaxCPU: 4, MaxRAMMb: 2048, MaxDiskGb: 10, MaxConcurrentJobs: 1},
		"no token":          {DatabasusHost: "https://x:4005", AgentID: "a", MaxCPU: 4, MaxRAMMb: 2048, MaxDiskGb: 10, MaxConcurrentJobs: 1},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := cfg.Validate()
			require.Error(t, err)
		})
	}
}

func Test_Validate_WhenStringsSetButCapacityInvalid_ReturnsCapacityError(t *testing.T) {
	cfg := &Config{
		DatabasusHost:     "https://primary.example:4005",
		AgentID:           "agent-1",
		Token:             "secret",
		MaxCPU:            2,
		MaxRAMMb:          4096,
		MaxDiskGb:         100,
		MaxConcurrentJobs: 4,
	}

	_, err := cfg.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "CPU per job")
}

func Test_DeriveCapacity_WhenConfigValid_DerivesPerJobSplit(t *testing.T) {
	cfg := &Config{MaxCPU: 8, MaxRAMMb: 4096, MaxDiskGb: 100, MaxConcurrentJobs: 4}

	capacity, err := cfg.DeriveCapacity()

	require.NoError(t, err)
	assert.Equal(t, 2, capacity.CPUPerJob)
	assert.Equal(t, 1024, capacity.RAMMbPerJob)
	assert.Equal(t, 8, capacity.MaxCPU)
	assert.Equal(t, 4096, capacity.MaxRAMMb)
	assert.Equal(t, 100, capacity.MaxDiskGb)
	assert.Equal(t, 4, capacity.MaxConcurrentJobs)
}

func Test_DeriveCapacity_WhenConcurrentJobsExceedCPU_ReturnsError(t *testing.T) {
	cfg := &Config{MaxCPU: 2, MaxRAMMb: 4096, MaxDiskGb: 100, MaxConcurrentJobs: 4}

	_, err := cfg.DeriveCapacity()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "CPU per job")
}

func Test_DeriveCapacity_WhenRAMBelowFloor_ReturnsError(t *testing.T) {
	cfg := &Config{MaxCPU: 4, MaxRAMMb: 256, MaxDiskGb: 100, MaxConcurrentJobs: 1}

	_, err := cfg.DeriveCapacity()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "max-ram-mb")
}

func Test_DeriveCapacity_WhenRAMPerJobBelowFloor_ReturnsError(t *testing.T) {
	cfg := &Config{MaxCPU: 8, MaxRAMMb: 1024, MaxDiskGb: 100, MaxConcurrentJobs: 8}

	_, err := cfg.DeriveCapacity()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "per job")
}

func Test_DeriveCapacity_WhenZeroFields_ReturnsError(t *testing.T) {
	for name, cfg := range map[string]*Config{
		"no concurrent jobs": {MaxCPU: 4, MaxRAMMb: 2048, MaxDiskGb: 10, MaxConcurrentJobs: 0},
		"no cpu":             {MaxCPU: 0, MaxRAMMb: 2048, MaxDiskGb: 10, MaxConcurrentJobs: 1},
		"no disk":            {MaxCPU: 4, MaxRAMMb: 2048, MaxDiskGb: 0, MaxConcurrentJobs: 1},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := cfg.DeriveCapacity()
			require.Error(t, err)
		})
	}
}

func Test_ValidateTransport_WhenHTTPS_PassesWithoutPrompt(t *testing.T) {
	cfg := &Config{DatabasusHost: "https://primary.example:4005"}

	err := cfg.ValidateTransport(false, emptyReader())

	require.NoError(t, err)
}

func Test_ValidateTransport_WhenHTTPNonTTYWithoutFlag_FailsNamingBothFixes(t *testing.T) {
	cfg := &Config{DatabasusHost: "http://primary.example:4005"}

	err := cfg.ValidateTransport(false, emptyReader())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "https://")
	assert.Contains(t, err.Error(), "--allow-insecure-http")
}

func Test_ValidateTransport_WhenHTTPWithAllowFlag_PassesWithWarn(t *testing.T) {
	cfg := &Config{DatabasusHost: "http://primary.example:4005", AllowInsecureHTTP: true}

	err := cfg.ValidateTransport(false, emptyReader())

	require.NoError(t, err)
}

func Test_ValidateTransport_WhenHTTPTTYAndOperatorConsents_Passes(t *testing.T) {
	cfg := &Config{DatabasusHost: "http://primary.example:4005"}

	err := cfg.ValidateTransport(true, bufio.NewReader(strings.NewReader("y\n")))

	require.NoError(t, err)
}

func Test_ValidateTransport_WhenHTTPTTYAndOperatorDeclines_ReturnsError(t *testing.T) {
	cfg := &Config{DatabasusHost: "http://primary.example:4005"}

	err := cfg.ValidateTransport(true, bufio.NewReader(strings.NewReader("n\n")))

	require.Error(t, err)
}

func Test_ValidateTransport_WhenSchemeUnsupported_ReturnsError(t *testing.T) {
	cfg := &Config{DatabasusHost: "ftp://primary.example:4005"}

	err := cfg.ValidateTransport(true, emptyReader())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "https://")
}

func Test_MaskSensitive_WhenEmpty_ReturnsNotSet(t *testing.T) {
	assert.Equal(t, "(not set)", maskSensitive(""))
}

func Test_MaskSensitive_WhenValue_RevealsQuarterThenMasks(t *testing.T) {
	assert.Equal(t, "ab***", maskSensitive("abcdefgh"))
}
