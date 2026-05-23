package postgresql_logical

type PostgresSslMode string

const (
	PostgresSslModeDisable    PostgresSslMode = "disable"
	PostgresSslModeRequire    PostgresSslMode = "require"
	PostgresSslModeVerifyCA   PostgresSslMode = "verify-ca"
	PostgresSslModeVerifyFull PostgresSslMode = "verify-full"
)
