all: clean tools test build gosec

clean:
	rm -rf build/
	rm -rf testab/

test:
	go test ./... -coverpkg=./... -count=1 -coverprofile test-coverage.out

build:
    # cd to directory where main.go exits, hack fix for go bug to embed version control data
    # https://github.com/golang/go/issues/51279
	cd ./cli/alphabill && go build -o ../../build/alphabill

gosec:
	gosec -fmt=sonarqube -out gosec_report.json -no-fail ./...

tools:
	go install github.com/securego/gosec/v2/cmd/gosec@latest

.PHONY: \
	all \
	clean \
	generate \
	tools \
	test \
	build \
	gosec
