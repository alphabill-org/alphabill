CC = clang
CXX = clang++
LD = wasm-ld-15

# Optional dependency from binaryen for smaller builds
WASM_OPT = wasm-opt
WASM_OPT_FLAGS = -Oz --zero-filled-memory --strip-producers

# Whether to build for debugging instead of release
DEBUG = 0

# Compilation flags
CFLAGS = -W -Wall -Wextra -Werror -Wno-unused -Wconversion -Wsign-conversion -MMD -MP -fno-exceptions --target=wasm32
ifeq ($(DEBUG), 1)
	CFLAGS += -DDEBUG -O0 -g
else
	CFLAGS += -DNDEBUG -Oz -flto
endif

# Linker flags
LDFLAGS = -zstack-size=14752 --no-entry --export=__heap_base --import-memory \
	--initial-memory=65536 --max-memory=1310720 --stack-first
ifeq ($(DEBUG), 1)
	LDFLAGS += --export-all --no-gc-sections
else
	LDFLAGS += --strip-all --gc-sections --lto-O3 -O3
endif

OBJECTS = $(patsubst src/%.c, build/%.o, $(wildcard src/*.c))
OBJECTS += $(patsubst src/%.cpp, build/%.o, $(wildcard src/*.cpp))
DEPS = $(OBJECTS:.o=.d)

MKDIR_BUILD = mkdir -p build
RMDIR = rm -rf

all: build/p2pkh_v2.wasm

# Link p2pkh_v2.wasm from all object files and run wasm-opt
build/p2pkh_v2.wasm: $(OBJECTS)
	$(LD) -o $@ $(OBJECTS) $(LDFLAGS)
ifneq ($(DEBUG), 1)
ifeq (, $(shell command -v $(WASM_OPT)))
	@echo Tip: $(WASM_OPT) was not found. Install 'wasm-opt' from 'https://github.com/WebAssembly/binaryen' for smaller builds!
else
	$(WASM_OPT) $(WASM_OPT_FLAGS) $@ -o $@
endif
endif

# Compile C sources
build/%.o: src/%.c
	@$(MKDIR_BUILD)
	$(CC) -c $< -o $@ $(CFLAGS)

# Compile C++ sources
build/%.o: src/%.cpp
	@$(MKDIR_BUILD)
	$(CXX) -c $< -o $@ $(CFLAGS)

.PHONY: clean
clean:
	$(RM) build/*

-include $(DEPS)
