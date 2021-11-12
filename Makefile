test:
	go test ./... -count=1 -coverprofile test-coverage.out

.PHONY: \
	test
