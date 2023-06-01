package program

const (
	ProgramDeploy = "pdeploy"
	ProgramCall   = "pcall"
)

type PDeployAttributes struct {
	_          struct{} `cbor:",toarray"`
	ProgModule []byte
	ProgParams []byte
}

type PCallAttributes struct {
	_         struct{} `cbor:",toarray"`
	FuncName  string
	InputData []byte
}
