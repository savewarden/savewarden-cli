package backup

import "fmt"

// RecognizePrevious checks the proof chain of ADR 0006 §4 that current,
// observed without following links with identity id, is the generation
// published by the operation of head: the head receipt, whose file has
// the SHA-256 headSum, and the journal j of that operation, terminal in
// published, whose receipt_sha256 is headSum and whose published identity
// is id. Comparing the content of current with head is the caller's last
// proof. An error names the first missing proof: current is then never
// "old", whatever its content.
func RecognizePrevious(head Receipt, headSum [32]byte, j Journal, id FileID) error {
	identity, receipt, published := j.Published()
	switch {
	case j.Op != head.Operation:
		return fmt.Errorf("the journal of %s is not that of the head receipt %s", j.Op, head.Operation)
	case !published:
		return fmt.Errorf("operation %s is not terminal in published", j.Op)
	case receipt != headSum:
		return fmt.Errorf("the published record of %s does not match its receipt file", j.Op)
	case id != identity:
		return fmt.Errorf("current is %s, not the directory %s that %s published", id, identity, j.Op)
	}
	return nil
}
