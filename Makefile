all: clean tools generate test gosec

clean:
	rm -rf internal/rpc/payment/*.pb.go

generate:
	go generate proto/generate.go
	find . -type f -name 'generate_test.go' -exec \
	go generate {} \;

test:
	go test ./... -count=1 -coverprofile test-coverage.out

gosec:
	gosec -fmt=sonarqube -out gosec_report.json -no-fail ./...

tools:
	go install github.com/vektra/mockery/v2
	go install github.com/golang/protobuf/protoc-gen-go
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc

.PHONY: \
	all
	clean
	generate
	tools
	test
	gosec
