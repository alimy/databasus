package walmath

import "fmt"

type LSN uint64

func (lsn LSN) String() string {
	return fmt.Sprintf("%X/%X", uint32(lsn>>32), uint32(lsn))
}

func ParseLSN(s string) (LSN, error) {
	var hi, lo uint32

	n, err := fmt.Sscanf(s, "%X/%X", &hi, &lo)
	if err != nil {
		return 0, fmt.Errorf("walmath: invalid LSN %q: %w", s, err)
	}
	if n != 2 {
		return 0, fmt.Errorf("walmath: invalid LSN %q: expected two hex parts, got %d", s, n)
	}

	return LSN(uint64(hi)<<32 | uint64(lo)), nil
}
