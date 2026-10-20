// Package migrator applies the SQL files in database/migrations to PostgreSQL.
//
// It replaces the Flyway CLI (a Java/Docker dependency) while staying
// compatible with it: it reads and writes Flyway's own flyway_schema_history
// table and Flyway's checksum algorithm, so a database migrated by Flyway
// continues seamlessly and the history stays readable by Flyway.
package migrator

import (
	"bufio"
	"hash/crc32"
	"io"
	"strings"
)

// Checksum computes Flyway's checksum of a migration script: a CRC32 over the
// UTF-8 bytes of each line, with line terminators removed and a leading BOM
// stripped, returned as a signed 32-bit integer (Flyway's ChecksumCalculator).
// Verified against 279 real flyway_schema_history rows.
func Checksum(r io.Reader) (int32, error) {
	br := bufio.NewReader(r)
	crc := crc32.NewIEEE()
	first := true
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 || err == nil {
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			// Java's readLine also treats a lone '\r' as a line terminator.
			for i, part := range strings.Split(line, "\r") {
				if first && i == 0 {
					part = strings.TrimPrefix(part, "\uFEFF")
					first = false
				}
				crc.Write([]byte(part)) //nolint:errcheck // hash writes never fail
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	return int32(crc.Sum32()), nil
}
