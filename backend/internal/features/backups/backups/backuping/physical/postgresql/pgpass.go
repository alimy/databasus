package backuping_physical_postgresql

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	postgresql_physical "databasus-backend/internal/features/databases/databases/postgresql/physical"
	postgresql_shared "databasus-backend/internal/features/databases/databases/postgresql/shared"
	"databasus-backend/internal/util/encryption"
	"databasus-backend/internal/util/tools"
)

// Credentials is the temp directory and per-invocation libpq credential files
// for one pg_basebackup or pg_receivewal exec: .pgpass + optional client
// certificate / client key / server CA. Remove() deletes the whole directory.
//
// Mirrors logical's CredentialFiles (databases/postgresql/logical/credentialfiles.go)
// — duplicated rather than extracted because there is no second consumer yet.
type Credentials struct {
	Dir            string
	PgpassPath     string
	ClientCertPath string
	ClientKeyPath  string
	RootCertPath   string
}

// WriteCredentials materializes p's connection credentials into a fresh 0700
// temp directory. password must already be decrypted; certificate fields are
// decrypted here via encryptor (no-op for plaintext input).
func WriteCredentials(
	p *postgresql_physical.PostgresqlPhysicalDatabase,
	password string,
	encryptor encryption.FieldEncryptor,
) (*Credentials, error) {
	dir, err := os.MkdirTemp(os.TempDir(), "pgphys_"+uuid.New().String())
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}

	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)

		return nil, fmt.Errorf("failed to set temporary directory permissions: %w", err)
	}

	creds := &Credentials{Dir: dir}

	if err := creds.writePgpass(p, password); err != nil {
		_ = os.RemoveAll(dir)

		return nil, err
	}

	if p.SslMode != postgresql_shared.PostgresSslModeDisable && p.SslMode != "" {
		if err := creds.writeCertFiles(p, encryptor); err != nil {
			_ = os.RemoveAll(dir)

			return nil, err
		}
	}

	return creds, nil
}

func (c *Credentials) Remove() {
	if c == nil || c.Dir == "" {
		return
	}

	_ = os.RemoveAll(c.Dir)
}

func (c *Credentials) writePgpass(
	p *postgresql_physical.PostgresqlPhysicalDatabase,
	password string,
) error {
	content := fmt.Sprintf("%s:%d:*:%s:%s",
		tools.EscapePgpassField(p.Host),
		p.Port,
		tools.EscapePgpassField(p.Username),
		tools.EscapePgpassField(password),
	)

	path := filepath.Join(c.Dir, ".pgpass")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write .pgpass file: %w", err)
	}

	c.PgpassPath = path

	return nil
}

func (c *Credentials) writeCertFiles(
	p *postgresql_physical.PostgresqlPhysicalDatabase,
	encryptor encryption.FieldEncryptor,
) error {
	var err error

	if c.ClientCertPath, err = c.writeCert("client.crt", p.SslClientCert, encryptor); err != nil {
		return fmt.Errorf("failed to write client certificate: %w", err)
	}

	if c.ClientKeyPath, err = c.writeCert("client.key", p.SslClientKey, encryptor); err != nil {
		return fmt.Errorf("failed to write client key: %w", err)
	}

	if c.RootCertPath, err = c.writeCert("root.crt", p.SslRootCert, encryptor); err != nil {
		return fmt.Errorf("failed to write server CA certificate: %w", err)
	}

	return nil
}

func (c *Credentials) writeCert(
	fileName, encryptedPEM string,
	encryptor encryption.FieldEncryptor,
) (string, error) {
	if encryptedPEM == "" {
		return "", nil
	}

	pem, err := decryptIfNeeded(encryptedPEM, encryptor)
	if err != nil {
		return "", err
	}

	path := filepath.Join(c.Dir, fileName)
	if err := os.WriteFile(path, []byte(pem), 0o600); err != nil {
		return "", err
	}

	return path, nil
}

func decryptIfNeeded(
	value string,
	encryptor encryption.FieldEncryptor,
) (string, error) {
	if encryptor == nil {
		return value, nil
	}

	return encryptor.Decrypt(value)
}
