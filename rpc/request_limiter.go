package rpc

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/time/rate"
)

type (
	RequestLimiter struct {
		rateLimit         int
		rateLimiter       *rate.Limiter
		requestsTokenCost map[string]int
		log               *slog.Logger
	}

	RequestTokenCost struct {
		request   string
		tokenCost int
	}
)

// NewRequestLimiter returns a new RequestLimiter. Requests are limited based of input cost. Overall limit is "highest cost * rateLimit".
func NewRequestLimiter(rateLimit int, tokenCosts []RequestTokenCost, obs Observability) *RequestLimiter {
	var rateLimiter *rate.Limiter
	requestsTokenCost := make(map[string]int)
	if rateLimit == 0 {
		rateLimiter = rate.NewLimiter(rate.Inf, 0)
	} else {
		maxCost := 0
		for _, item := range tokenCosts {
			requestsTokenCost[item.request] = item.tokenCost
			if maxCost < item.tokenCost {
				maxCost = item.tokenCost
			}
		}
		limit := maxCost * rateLimit
		rateLimiter = rate.NewLimiter(rate.Limit(limit), limit)
	}

	return &RequestLimiter{
		rateLimit:         rateLimit,
		rateLimiter:       rateLimiter,
		requestsTokenCost: requestsTokenCost,
		log:               obs.Logger(),
	}
}

func (l *RequestLimiter) CheckRequestAllowed(request string) error {
	tokenCost, ok := l.requestsTokenCost[request]
	if !ok {
		l.log.Warn(fmt.Sprintf("Request %s not limited", request))
		return nil
	}
	if !l.rateLimiter.AllowN(time.Now(), tokenCost) {
		return errors.New("too many request")
	}
	return nil
}
