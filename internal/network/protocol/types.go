package protocol

// SystemIdentifier string representation of system identifier bytes
type SystemIdentifier string

func (id SystemIdentifier) Bytes() []byte {
	return []byte(id)
}
