all: tools generate test build build_scripts gosec

clean:
	rm -rf build/
	rm -rf testab/
	rm -rf internal/rpc/payment/*.pb.go

generate:
	go generate proto/generate.go

test:
	go test ./... -coverpkg=./... -count=1 -coverprofile test-coverage.out

build:
    # cd to directory where main.go exits, hack fix for go bug to embed version control data
    # https://github.com/golang/go/issues/51279
	cd ./cli/alphabill && go build -o ../../build/alphabill

build_scripts:
	go build -o build/alphabill-spend-initial-bill scripts/money/spend_initial_bill.go

gosec:
	gosec -fmt=sonarqube -out gosec_report.json -no-fail ./...

tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc
	go install github.com/securego/gosec/v2/cmd/gosec@latest

.PHONY: \
	all \
	clean \
	generate \
	tools \
	test \
	build \
	gosec
