test:
	go test ./... -count=1 -coverprofile test-coverage.out

gosec:
	gosec -fmt=sonarqube -out gosec_report.json -no-fail ./...

.PHONY: \
	test
	gosec
