//
// Alphabil host interface

#![allow(unused)]

/// P2PKH
pub fn p2pkh_v1() -> i32 {
    unsafe {
        extern_p2pkh_v1()
    }
}
extern "C" {
    #[link_name = "p2pkh_v1"]
    fn extern_p2pkh_v1() ->i32;
}

/// P2PKH_V2
pub fn p2pkh_v2(pub_key_hash: u64) -> i32 {
    unsafe {
        extern_p2pkh_v2(pub_key_hash)
    }
}
extern "C" {
    #[link_name = "p2pkh_v2"]
    fn extern_p2pkh_v2(pub_key_hash: u64) ->i32;
}


/// Log
pub fn log(level: u32, msg: u64) {
    unsafe {
        extern_log_v1(level, msg);
    }
}
extern "C" {
    #[link_name = "log_v1"]
    fn extern_log_v1(level: u32, msg: u64);
}

// ┌───────────────────────────────────────────────────────────────────────────┐
// │                                                                           │
// │ Memory                                                                    │
// │                                                                           │
// └───────────────────────────────────────────────────────────────────────────┘

/// Malloc
pub fn malloc(size: u32) -> u32 {
    unsafe { extern_malloc(size) }
}
extern "C" {
    #[link_name = "malloc"]
    fn extern_malloc(size: u32) -> u32;
}

/// Free
pub fn free(addr: u32) {
    unsafe { extern_free(addr) }
}
extern "C" {
    #[link_name = "free"]
    fn extern_free(addr: u32);
}

// ┌───────────────────────────────────────────────────────────────────────────┐
// │                                                                           │
// │ Storage Functions                                                         │
// │                                                                           │
// └───────────────────────────────────────────────────────────────────────────┘

pub fn store_data(id: u32, data_ptr: u64) -> i32 {
    unsafe { extern_storage_write(id, data_ptr) }
}
extern "C" {
    #[link_name = "storage_write"]
    fn extern_storage_write(id: u32, data_ptr: u64) -> i32;
}

pub fn read_data(id: u32) -> u64 {
    unsafe { extern_storage_read(id) }
}
extern "C" {
    #[link_name = "storage_read"]
    fn extern_storage_read(id: u32) -> u64;
}