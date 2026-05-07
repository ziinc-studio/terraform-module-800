package provider

import "strconv"

// fmtScan is a tiny shim so provider.go does not pull in the fmt package
// just to parse an int64 from a string.
func fmtScan(s string, v *int64) (int, error) {
	parsed, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	*v = parsed
	return 1, nil
}
