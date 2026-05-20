package container

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"databasus-verification-agent/internal/testutil"
)

func Test_BuildSpec_AppliesHardeningControls(t *testing.T) {
	// nil engine: buildSpec is pure and never calls the engine.
	containerManager := NewManager(nil, "agent-1", testutil.DiscardLogger())

	labels := map[string]string{LabelAgentID: "agent-1"}
	spec := containerManager.buildSpec(spawnPlan{
		verificationID: uuid.New(),
		image:          "postgres@sha256:x",
		password:       "pw",
		cpuPerJob:      2,
		ramMbPerJob:    1024,
		networkID:      "net-id",
		labels:         labels,
	})

	assert.True(t, spec.NoNewPrivileges)
	assert.True(t, spec.CapDropAll)
	assert.Equal(t, minimalCaps, spec.CapAdd)
	assert.EqualValues(t, containerPidsLimit, spec.PidsLimit)
	assert.Equal(t, "net-id", spec.NetworkID)
	assert.Equal(t, int64(2)*1_000_000_000, spec.NanoCPUs)
	assert.Equal(t, int64(1024)*1024*1024, spec.MemoryBytes)
	assert.Equal(t, []string{"POSTGRES_PASSWORD=pw"}, spec.Env)
}

func Test_GetInContainerConn_UsesInternalPort(t *testing.T) {
	c := &PostgresContainer{password: "pw"}

	conn := c.GetInContainerConn()

	assert.Equal(t, "127.0.0.1", conn.Host)
	assert.Equal(t, pgInternalPort, conn.Port)
	assert.Equal(t, restoreUser, conn.User)
	assert.Equal(t, restoreDB, conn.Database)
	assert.Equal(t, "pw", conn.Password)
}

func Test_GetVerifierConn_WhenSpawned_UsesResolvedHostPort(t *testing.T) {
	c := &PostgresContainer{password: "pw", hostPort: 54321}

	conn := c.GetVerifierConn()

	assert.Equal(t, "127.0.0.1", conn.Host)
	assert.Equal(t, 54321, conn.Port)
	assert.Equal(t, restoreUser, conn.User)
	assert.Equal(t, restoreDB, conn.Database)
	assert.Equal(t, "pw", conn.Password)
}

func Test_ImageForMajor_ReturnsPostgresOfficialTagPerMajor(t *testing.T) {
	cases := []struct {
		major    string
		expected string
	}{
		{"12", "postgres:12"},
		{"13", "postgres:13"},
		{"14", "postgres:14"},
		{"15", "postgres:15"},
		{"16", "postgres:16"},
		{"17", "postgres:17"},
		{"18", "postgres:18"},
	}

	for _, tc := range cases {
		t.Run(tc.major, func(t *testing.T) {
			assert.Equal(t, tc.expected, imageForMajor(tc.major))
		})
	}
}
