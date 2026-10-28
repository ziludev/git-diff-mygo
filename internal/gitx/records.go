package gitx

import "strings"

// splitRecords splits output written with NUL between records and %1f between
// fields. NUL is the only safe record separator: a commit subject is arbitrary
// text and may contain any other control byte. fields is the number of fields
// per record; the last field keeps any stray separator, so a subject may
// contain one.
func splitRecords(out []byte, fields int) [][]string {
	records := [][]string{}
	for _, record := range strings.Split(string(out), "\x00") {
		// git ends every record with a newline after the separator.
		record = strings.Trim(record, "\n")
		if record == "" {
			continue
		}
		parts := strings.SplitN(record, "\x1f", fields)
		if len(parts) != fields {
			continue
		}
		records = append(records, parts)
	}
	return records
}
