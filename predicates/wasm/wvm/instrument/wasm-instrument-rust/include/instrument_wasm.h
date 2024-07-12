#include <stdarg.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>

typedef struct MemoryBuffer {
  size_t size;
  unsigned char *data;
} MemoryBuffer;

int32_t instrument_wasm(const struct MemoryBuffer *wasm,
                        uint32_t stack_limit,
                        struct MemoryBuffer *out);

char *errstr(void);

void errstr_free(char *err_str);

void memory_buffer_new(struct MemoryBuffer *out, uintptr_t size, const unsigned char *ptr);

void memory_buffer_delete(struct MemoryBuffer *buf);
