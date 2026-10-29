package backup

// Severity says whether a finding makes a result untrustworthy.
type Severity int

// Severities.
const (
	Info Severity = iota
	Blocking
)

func (s Severity) String() string {
	if s == Blocking {
		return "blocking"
	}
	return "info"
}

// Code identifies a kind of finding. Codes are stable for scripts; each
// feature defines the codes it emits.
type Code string

// Finding is an anomaly met while inventorying or planning.
type Finding struct {
	Severity Severity
	Code     Code
	Source   string // source id, empty when not tied to a source
	// Path is relative to the source with slashes, or a Windows path when
	// the problem lies on the way to it.
	Path   string
	Detail string
}

// CountBlocking counts the blocking findings.
func CountBlocking(findings []Finding) int {
	count := 0
	for _, finding := range findings {
		if finding.Severity == Blocking {
			count++
		}
	}
	return count
}
