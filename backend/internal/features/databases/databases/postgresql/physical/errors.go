package postgresql_physical

import (
	"fmt"
	"strings"
)

type UnsupportedTablespacesError struct {
	Spcnames []string
}

func (e *UnsupportedTablespacesError) Error() string {
	return fmt.Sprintf(
		"physical backups do not support custom tablespaces (found: %s); drop these tablespaces or switch the database to logical backups",
		strings.Join(e.Spcnames, ", "),
	)
}
