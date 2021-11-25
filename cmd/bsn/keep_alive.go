package main

import (
	"time"

	grpckeepalive "google.golang.org/grpc/keepalive"
)

// TODO add to the grpc server
type (
	// If needed for other components as well, move to internal/cli/common_conf.go
	keepAliveServerConfiguration struct {
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

func (c *keepAliveServerConfiguration) GrpcKeepAliveServerParameters() (grpckeepalive.ServerParameters, error) {
	p := grpckeepalive.ServerParameters{}
	if c.MaxConnectionAgeMs != nil {
		p.MaxConnectionAge = time.Duration(*c.MaxConnectionAgeMs) * time.Millisecond
		log.Debug("Server grpc client connection max connection age: %dms", *c.MaxConnectionAgeMs)
	}
	if c.MaxConnectionAgeGraceMs != nil {
		p.MaxConnectionAgeGrace = time.Duration(*c.MaxConnectionAgeGraceMs) * time.Millisecond
		log.Debug("Server grpc client connection max connection age grace: %dms", *c.MaxConnectionAgeGraceMs)
	}
	return p, nil
}
