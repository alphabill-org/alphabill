all: build copy

clean:
	-rm -rf target/

install-targets:
	rustup target add aarch64-unknown-linux-gnu
	rustup target add x86_64-unknown-linux-gnu
	rustup target add aarch64-apple-darwin
	rustup target add x86_64-apple-darwin
	rustup target add x86_64-pc-windows-gnu

build: install-targets
	cargo build --release --target aarch64-unknown-linux-gnu
	cargo build --release --target x86_64-unknown-linux-gnu
	cargo build --release --target x86_64-apple-darwin
	cargo build --release --target aarch64-apple-darwin
copy:
	cp -f target/aarch64-unknown-linux-gnu/release/libwasm_instrument.a lib/aarch64-unknown-linux-gnu/
	cp -f target/x86_64-unknown-linux-gnu/release/libwasm_instrument.a lib/x86_64-unknown-linux-gnu/
	cp -f target/x86_64-apple-darwin/release/libwasm_instrument.a lib/x86_64-apple-darwin/
	cp -f target/aarch64-apple-darwin/release/libwasm_instrument.a lib/aarch64-apple-darwin/

.PHONY: \
	all \
	clean \
	install-targets \
	build \
	copy
