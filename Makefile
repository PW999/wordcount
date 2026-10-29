REQUIRED_TOOLS := cmake g++ make git go

WHISPER_TAG  := v1.9.5
WHISPER_REPO := https://github.com/ggml-org/whisper.cpp
ROOT         := $(CURDIR)
WHISPER_DIR  := $(ROOT)/third_party/whisper.cpp
BUILD_DIR    := $(ROOT)/build

# GPU selects the compute backend: empty for the CPU-only default, or "vulkan".
# Each backend builds into its own directory so switching needs no clean rebuild.
GPU ?=
ifeq ($(GPU),)
WHISPER_BUILD := $(BUILD_DIR)/whisper
GPU_CMAKE_FLAGS :=
GPU_LDFLAGS :=
GPU_GOFLAGS :=
else ifeq ($(GPU),vulkan)
WHISPER_BUILD := $(BUILD_DIR)/whisper-vulkan
GPU_CMAKE_FLAGS := -DGGML_VULKAN=1
GPU_LDFLAGS := -L$(WHISPER_BUILD)/ggml/src/ggml-vulkan -lggml-vulkan -lvulkan
GPU_GOFLAGS := -tags=vulkan
else
$(error unknown GPU=$(GPU); use GPU=vulkan or leave it empty for a CPU-only build)
endif
WHISPER_LIB  := $(WHISPER_BUILD)/src/libwhisper.a
PATCHES      := $(sort $(wildcard $(ROOT)/patches/*.patch))
PATCH_STAMP  := $(WHISPER_DIR)/.wordcount-patched

# whisper/ggml are linked statically; the C++ runtime (libstdc++, libgcc_s,
# libgomp) stays dynamic because the Go bindings hard-code -lstdc++ -fopenmp.
export CGO_CFLAGS  := -I$(WHISPER_DIR)/include -I$(WHISPER_DIR)/ggml/include
export GOFLAGS := $(strip $(GOFLAGS) $(GPU_GOFLAGS))
export CGO_LDFLAGS := -L$(WHISPER_BUILD)/src -L$(WHISPER_BUILD)/ggml/src $(GPU_LDFLAGS)

.PHONY: check-deps check-gpu-deps whisper build test clean

check-deps:
	@missing=""; \
	for tool in $(REQUIRED_TOOLS); do \
		if ! command -v $$tool >/dev/null 2>&1; then missing="$$missing $$tool"; fi; \
	done; \
	if [ -n "$$missing" ]; then \
		echo "error: missing build prerequisites:$$missing" >&2; \
		echo "install them first, e.g. on Fedora: sudo dnf install cmake gcc-c++ make git golang" >&2; \
		exit 1; \
	fi; \
	echo "all build prerequisites found ($(REQUIRED_TOOLS))"

# Only the GPU build needs these; the check runs before anything compiles so a
# missing package never leaves a half-built tree or replaces a working binary.
check-gpu-deps:
ifeq ($(GPU),vulkan)
	@missing=""; \
	[ -f /usr/include/vulkan/vulkan.h ] || missing="$$missing vulkan-headers(/usr/include/vulkan/vulkan.h)"; \
	command -v glslc >/dev/null 2>&1 || missing="$$missing glslc"; \
	[ -f /usr/include/spirv/unified1/spirv.hpp ] || [ -f /usr/include/spirv/1.0/spirv.hpp ] || missing="$$missing spirv-headers"; \
	if [ -n "$$missing" ]; then \
		echo "error: missing GPU build prerequisites:$$missing" >&2; \
		echo "install them first, e.g. on Fedora: sudo dnf install vulkan-headers vulkan-loader-devel glslc spirv-headers-devel" >&2; \
		exit 1; \
	fi; \
	echo "all GPU build prerequisites found (vulkan)"
endif

$(WHISPER_DIR)/CMakeLists.txt:
	git clone --depth 1 --branch $(WHISPER_TAG) $(WHISPER_REPO) $(WHISPER_DIR)

# Patches to the cloned Go binding (see patches/). Applied once; a patch that is
# already applied is skipped, and one that no longer applies (for example after
# bumping WHISPER_TAG) fails the build instead of silently dropping GPU support.
$(PATCH_STAMP): $(WHISPER_DIR)/CMakeLists.txt $(PATCHES)
	@set -e; for p in $(PATCHES); do \
		if git -C $(WHISPER_DIR) apply --reverse --check $$p 2>/dev/null; then \
			echo "patch $$(basename $$p) already applied"; \
		elif git -C $(WHISPER_DIR) apply --check $$p 2>/dev/null; then \
			git -C $(WHISPER_DIR) apply $$p; echo "applied patch $$(basename $$p)"; \
		else \
			echo "error: patches/$$(basename $$p) does not apply to whisper.cpp $(WHISPER_TAG); update the patch or WHISPER_TAG" >&2; \
			exit 1; \
		fi; \
	done
	@touch $@

$(WHISPER_LIB): check-deps check-gpu-deps $(PATCH_STAMP)
	cmake -S $(WHISPER_DIR) -B $(WHISPER_BUILD) \
		-DCMAKE_BUILD_TYPE=Release \
		-DBUILD_SHARED_LIBS=OFF \
		-DGGML_OPENMP=OFF \
		$(GPU_CMAKE_FLAGS) \
		-DWHISPER_BUILD_EXAMPLES=OFF \
		-DWHISPER_BUILD_TESTS=OFF
	cmake --build $(WHISPER_BUILD) --target whisper -j

whisper: $(WHISPER_LIB)

build: $(WHISPER_LIB)
	go build -o wordcount ./cmd/wordcount

test: $(WHISPER_LIB)
	go vet ./...
	go test -race -count=1 ./...

clean:
	rm -rf $(BUILD_DIR) wordcount
