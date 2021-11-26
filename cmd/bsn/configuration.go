package main

type (
	configuration struct {
		Server           ServerConfiguration
		InitialBillValue uint32 `validate:"gte=0"` // The value of initial bill in AlphaBills.
	}

	ServerConfiguration struct {
		Address        string `validate:"empty=false"`            // Listen address together with port.
		MaxRecvMsgSize int    `validate:"gte=0" default:"256000"` // Maximum number of bytes the incoming message may be.
	}
)
