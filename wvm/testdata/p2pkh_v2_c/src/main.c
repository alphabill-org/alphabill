#include "ab_env.h"

uint64_t pack_ptr_and_len(uint32_t ptr, uint32_t len){
	return (((uint64_t)len<<32) | ptr);
}

WASM_EXPORT("run")
int32_t run(uint64_t pub_key_hash_ptr) {
	uint32_t len = (uint32_t)(pub_key_hash_ptr>>32);
	// check value length
	if (len != 32) {
		return -1;
	}
	uint32_t msg_len = 3;
	uint32_t addr = ab_malloc(msg_len);
	uint8_t *p_msg = (uint8_t *) addr;
	p_msg[0] = 'f';
	p_msg[1] = 'u';
	p_msg[2] = 'n';

	ext_log_v1(LOG_INFO, pack_ptr_and_len(addr, msg_len));
	return ext_p2pkh_v2(pub_key_hash_ptr);
}