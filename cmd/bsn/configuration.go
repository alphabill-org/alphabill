package main

type (
	configuration struct {
		Server           ServerConfiguration
		InitialBillValue int `validate:"gte=0"` // The value of initial bill in AlphaBills.
	}

	ServerConfiguration struct {
		Address        string `validate:"empty=false"` // Listen address together with port.
		MaxRecvMsgSize int    `validate:"gte=0"`
	}
)
