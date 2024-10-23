MACOSSDK=https://github.com/roblabla/MacOSX-SDKs/releases/download/macosx14.0/MacOSX14.0.sdk.tar.xz
UNAME_S := $(shell uname -s)

all: build copy

download-sdk:
        ifeq ($(UNAME_S),Linux)
            ifeq ($(wildcard MacOS*),)
		@echo "MAC SDK does not exist. Dowloading..."
		curl -L "$(MACOSSDK)" | tar xJ --warning=no-unknown-keyword
            else
		@echo "SDK is installed"
            endif
        endif 
clean:
	-rm -rf target/
	-rm -rf MacOSX*

install-targets:
	rustup target add aarch64-unknown-linux-gnu
	rustup target add x86_64-unknown-linux-gnu
	rustup target add aarch64-apple-darwin
	rustup target add x86_64-apple-darwin
	rustup target add x86_64-pc-windows-gnu

build: install-targets download-sdk
	cargo build --release --target aarch64-unknown-linux-gnu
	cargo build --release --target x86_64-unknown-linux-gnu
        ifeq ($(UNAME_S),Linux)
		SDKROOT=$(shell pwd)/MacOSX14.0.sdk/ \
		CARGO_TARGET_X86_64_APPLE_DARWIN_LINKER=rust-lld \
		cargo build --release --target x86_64-apple-darwin
		SDKROOT=$(shell pwd)/MacOSX14.0.sdk/ \
		CARGO_TARGET_X86_64_APPLE_DARWIN_LINKER=rust-lld \
		cargo build --release --target aarch64-apple-darwin
        endif
        ifeq ($(UNAME_S),Darwin)
		cargo build --release --target x86_64-apple-darwin
		cargo build --release --target aarch64-apple-darwin
        endif
copy:
	cp -f target/aarch64-unknown-linux-gnu/release/libwasm_instrument.a lib/aarch64-unknown-linux-gnu/
	cp -f target/x86_64-unknown-linux-gnu/release/libwasm_instrument.a lib/x86_64-unknown-linux-gnu/
	cp -f target/x86_64-apple-darwin/release/libwasm_instrument.a lib/x86_64-apple-darwin/
	cp -f target/aarch64-apple-darwin/release/libwasm_instrument.a lib/aarch64-apple-darwin/

.PHONY: \
	all \
	download-sdk\
	clean \
	install-targets \
	build \
	copy
