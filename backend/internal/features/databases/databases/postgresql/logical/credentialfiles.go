package postgresql_logical

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"databasus-backend/internal/util/encryption"
	"databasus-backend/internal/util/tools"
)

// CredentialFiles is the temporary directory and files libpq needs for one
// pg_dump / pg_restore / pgx invocation: a .pgpass file plus the optional client
// certificate, client key, and server CA certificate. Remove the whole directory
// to clean every file up at once.
type CredentialFiles struct {
	Dir            string
	PgpassPath     string
	ClientCertPath string
	ClientKeyPath  string
	RootCertPath   string
}

// WriteCredentialFiles materializes the connection credentials of p into a fresh
// 0700 temp directory. password must already be decrypted; certificate fields are
// decrypted here via encryptor (decryption is a no-op for plaintext input, so this
// also works for the restore target whose config is never persisted).
func WriteCredentialFiles(
	p *PostgresqlLogicalDatabase,
	password string,
	encryptor encryption.FieldEncryptor,
) (*CredentialFiles, error) {
	// Credential files use the OS temp dir because some filesystems
	// (e.g. ZFS on TrueNAS) ignore chmod, causing "group or world access" errors.
	dir, err := os.MkdirTemp(os.TempDir(), "pgcreds_"+uuid.New().String())
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}

	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("failed to set temporary directory permissions: %w", err)
	}

	files := &CredentialFiles{Dir: dir}

	if err := files.writePgpass(p, password); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}

	if p.SslMode != PostgresSslModeDisable && p.SslMode != "" {
		if err := files.writeCertFiles(p, encryptor); err != nil {
			_ = os.RemoveAll(dir)
			return nil, err
		}
	}

	return files, nil
}

// Remove deletes the temp directory and every credential file inside it.
func (f *CredentialFiles) Remove() {
	if f == nil || f.Dir == "" {
		return
	}

	_ = os.RemoveAll(f.Dir)
}

func (f *CredentialFiles) writePgpass(p *PostgresqlLogicalDatabase, password string) error {
	content := fmt.Sprintf("%s:%d:*:%s:%s",
		tools.EscapePgpassField(p.Host),
		p.Port,
		tools.EscapePgpassField(p.Username),
		tools.EscapePgpassField(password),
	)

	path := filepath.Join(f.Dir, ".pgpass")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write .pgpass file: %w", err)
	}

	f.PgpassPath = path

	return nil
}

func (f *CredentialFiles) writeCertFiles(
	p *PostgresqlLogicalDatabase,
	encryptor encryption.FieldEncryptor,
) error {
	var err error

	if f.ClientCertPath, err = f.writeCert("client.crt", p.SslClientCert, encryptor); err != nil {
		return fmt.Errorf("failed to write client certificate: %w", err)
	}

	if f.ClientKeyPath, err = f.writeCert("client.key", p.SslClientKey, encryptor); err != nil {
		return fmt.Errorf("failed to write client key: %w", err)
	}

	if f.RootCertPath, err = f.writeCert("root.crt", p.SslRootCert, encryptor); err != nil {
		return fmt.Errorf("failed to write server CA certificate: %w", err)
	}

	return nil
}

func (f *CredentialFiles) writeCert(
	fileName, encryptedPEM string,
	encryptor encryption.FieldEncryptor,
) (string, error) {
	if encryptedPEM == "" {
		return "", nil
	}

	pem, err := decryptFieldIfNeeded(encryptedPEM, encryptor)
	if err != nil {
		return "", err
	}

	path := filepath.Join(f.Dir, fileName)
	if err := os.WriteFile(path, []byte(pem), 0o600); err != nil {
		return "", err
	}

	return path, nil
}

// openPgConn writes p's credential files, opens a pgx connection to dbName, and
// removes the files once the TLS handshake has completed.
func openPgConn(
	ctx context.Context,
	p *PostgresqlLogicalDatabase,
	dbName string,
	encryptor encryption.FieldEncryptor,
) (*pgx.Conn, error) {
	password, err := decryptFieldIfNeeded(p.Password, encryptor)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt password: %w", err)
	}

	files, err := WriteCredentialFiles(p, password, encryptor)
	if err != nil {
		return nil, err
	}
	defer files.Remove()

	return pgx.Connect(ctx, buildConnectionStringForDB(p, dbName, password, files))
}

// buildConnectionStringForDB builds a libpq connection string for a specific database.
func buildConnectionStringForDB(
	p *PostgresqlLogicalDatabase,
	dbName, password string,
	files *CredentialFiles,
) string {
	sslMode := p.SslMode
	if sslMode == "" {
		sslMode = PostgresSslModeDisable
	}

	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password='%s' dbname=%s sslmode=%s default_query_exec_mode=simple_protocol standard_conforming_strings=on client_encoding=UTF8",
		p.Host,
		p.Port,
		p.Username,
		escapeConnectionStringValue(password),
		dbName,
		sslMode,
	)

	if files != nil {
		if files.ClientCertPath != "" {
			connStr += fmt.Sprintf(" sslcert='%s'", escapeConnectionStringValue(files.ClientCertPath))
		}

		if files.ClientKeyPath != "" {
			connStr += fmt.Sprintf(" sslkey='%s'", escapeConnectionStringValue(files.ClientKeyPath))
		}

		if files.RootCertPath != "" {
			connStr += fmt.Sprintf(" sslrootcert='%s'", escapeConnectionStringValue(files.RootCertPath))
		}
	}

	return connStr
}

func escapeConnectionStringValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)

	return value
}

func decryptFieldIfNeeded(
	value string,
	encryptor encryption.FieldEncryptor,
) (string, error) {
	if encryptor == nil {
		return value, nil
	}

	return encryptor.Decrypt(value)
}
