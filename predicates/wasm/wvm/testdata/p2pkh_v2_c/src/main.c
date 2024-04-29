#include "ab_env.h"
#include "nanoprintf.h"

uint64_t pack_ptr_and_len(uint32_t ptr, uint32_t len){
	return (((uint64_t)len<<32) | ptr);
}

WASM_EXPORT("run")
int32_t run(uint64_t pub_key_hash_ptr) {
	uint32_t hash_len = (uint32_t)(pub_key_hash_ptr>>32);
	uint8_t *hash_p = (uint8_t *)(uint32_t)(pub_key_hash_ptr & 0xFFFFFFFF);
	// check value length
	if (hash_len != 32) {
		return -1;
	}
	size_t bufSize = 256;
	uint32_t addr = ab_malloc(bufSize);
	char *msg_p = (char *) addr;
	int msg_len = npf_snprintf(msg_p, bufSize, "PubKey hash: ");
	for (size_t i = 0; i < hash_len; i++) {
		msg_len += npf_snprintf(msg_p+msg_len, bufSize, "%.2x", hash_p[i]);
	}
	ext_log_v1(LOG_INFO, pack_ptr_and_len(addr, (uint32_t )msg_len));
	return ext_p2pkh_v2(pub_key_hash_ptr);
}