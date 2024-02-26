use host_api::utils::unpack_ptr_and_len;
use host_api::env::p2pkh_v2;
use host_api::env::log;


#[no_mangle]
fn run(pub_key_hash: u64) -> i32 {
  let (ptr, len) = unpack_ptr_and_len(pub_key_hash);
  if len == 32{
    p2pkh_v2(pub_key_hash);
  }
  let vec_of_five = vec![1, 2, 3, 4, 5];
  0
}
