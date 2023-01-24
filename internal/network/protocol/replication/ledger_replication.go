package replication

import "fmt"

func (r *LedgerReplicationResponse) Pretty() string {
	result := "status: " + r.Status.String()
	if r.Message != "" {
		result += ", message: " + r.Message
	} else {
		count := len(r.Blocks)
		if count > 0 {
			result += fmt.Sprintf(", blocks %d..%d", r.Blocks[0].UnicityCertificate.InputRecord.RoundNumber, r.Blocks[count-1].UnicityCertificate.InputRecord.RoundNumber)
		}
	}
	return result
}
