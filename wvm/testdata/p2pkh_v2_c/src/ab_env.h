//
// Alphabill host interface

#pragma once

#include <stdint.h>

#define WASM_EXPORT(name) __attribute__((export_name(name)))
#define WASM_IMPORT(name) __attribute__((import_name(name)))

WASM_IMPORT("p2pkh_v1")
int32_t ext_p2pkh_v1 ();

WASM_IMPORT("p2pkh_v2")
int32_t ext_p2pkh_v2 (uint64_t pub_key_hash_ptr);

#define LOG_ERROR 0
#define LOG_WARN  1
#define LOG_INFO  2
#define LOG_DEBUG 3
WASM_IMPORT("log_v1")
void ext_log_v1 (uint32_t level, uint64_t msg_ptr);

// ┌───────────────────────────────────────────────────────────────────────────┐
// │                                                                           │
// │ Memory                                                                    │
// │                                                                           │
// └───────────────────────────────────────────────────────────────────────────┘

/** Copies pixels to the framebuffer. */
WASM_IMPORT("ext_malloc")
uint32_t ab_malloc (uint32_t size);

/** Copies a subregion within a larger sprite atlas to the framebuffer. */
WASM_IMPORT("ext_free")
void ab_free (uint32_t address);

// ┌───────────────────────────────────────────────────────────────────────────┐
// │                                                                           │
// │ Storage Functions                                                         │
// │                                                                           │
// └───────────────────────────────────────────────────────────────────────────┘

/** Reads up to `size` bytes from persistent storage into the pointer `dest`. */
WASM_IMPORT("storage_write")
int32_t store_data (uint32_t id, uint64_t data_ptr);

/** Writes up to `size` bytes from the pointer `src` into persistent storage. */
WASM_IMPORT("storage_read")
uint64_t read_data (uint32_t id);