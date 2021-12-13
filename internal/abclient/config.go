package abclient

type Config struct {
	AlphaBill *AlphaBillClientConfig
}

type AlphaBillClientConfig struct {
	Uri string
}
