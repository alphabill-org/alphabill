package main

import (
	"time"

	grpckeepalive "google.golang.org/grpc/keepalive"
)

type (
	configuration struct {
		Server           ServerConfiguration
		InitialBillValue uint32 `validate:"gte=0"` // The value of initial bill in AlphaBills.
	}

	ServerConfiguration struct {
		// Listen address together with port.
		Address string `validate:"empty=false"`

		// Maximum number of bytes the incoming message may be.
		MaxRecvMsgSize int `validate:"gte=0" default:"256000"`

		// MaxConnectionAgeMs is a duration for the maximum amount of time a
		// connection may exist before it will be closed by sending a GoAway. A
		// random jitter of +/-10% will be added to MaxConnectionAgeMs to spread out
		// connection storms.
		MaxConnectionAgeMs *int64

		// MaxConnectionAgeGraceMs is an additive period after MaxConnectionAgeMs after
		// which the connection will be forcibly closed.
		MaxConnectionAgeGraceMs *int64
	}
)

func (c *ServerConfiguration) GrpcKeepAliveServerParameters() grpckeepalive.ServerParameters {
	p := grpckeepalive.ServerParameters{}
	if c.MaxConnectionAgeMs != nil {
		p.MaxConnectionAge = time.Duration(*c.MaxConnectionAgeMs) * time.Millisecond
		log.Debug("Server grpc client connection max connection age: %dms", *c.MaxConnectionAgeMs)
	}
	if c.MaxConnectionAgeGraceMs != nil {
		p.MaxConnectionAgeGrace = time.Duration(*c.MaxConnectionAgeGraceMs) * time.Millisecond
		log.Debug("Server grpc client connection max connection age grace: %dms", *c.MaxConnectionAgeGraceMs)
	}
	return p
}
