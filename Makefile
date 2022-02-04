all: clean tools generate test gosec

clean:
	rm -rf internal/rpc/*/*.pb.go

generate:
	go generate proto/generate.go

test:
	go test ./... -count=1 -coverprofile test-coverage.out

gosec:
	gosec -fmt=sonarqube -out gosec_report.json -no-fail ./...

tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go
	go install github.com/securego/gosec/v2/cmd/gosec@latest

.PHONY: \
	all
	clean
	generate
	tools
	test
	gosec
