package script

const (
	OpDup        = 0x76
	OpHash       = 0xa8
	OpPushHash   = 0x4f
	OpPushPubKey = 0x55
	OpPushSig    = 0x54
	OpCheckSig   = 0xac
	OpEqual      = 0x87
	OpVerify     = 0x69
	OpPushBool   = 0x51
)

const (
	StartByte          = 0x53
	HashAlgSha256      = 0x01
	HashAlgSha512      = 0x02
	BoolFalse          = 0x00
	BoolTrue           = 0x01
	SigSchemeSecp256k1 = 0x01
)
