#![no_std]
 
/// This function is called on panic.
// Need to provide a tiny `panic` implementation for `#![no_std]`.
// This translates into an `unreachable` instruction that will
// raise a `trap` the WebAssembly execution if we panic at runtime.

#[cfg(not(test))]
#[panic_handler]
fn panic(_panic: &core::panic::PanicInfo<'_>) -> ! {
    core::arch::wasm32::unreachable()
}

#[link(wasm_import_module = "ab")]
extern "C" {
    /// WebAssembly import which prints a string (linear memory offset,
    /// byteCount) to the console.
    ///
    /// Note: This is not an ownership transfer: Rust still owns the pointer
    /// and ensures it isn't deallocated during this call.
    #[link_name = "get_state"]
    fn _get_state(id: u32, ptr: u32, size: u32) -> i32;

    #[link_name = "set_state"]
    fn _set_state(id: u32, ptr: u32, size: u32) -> i32;

    #[link_name = "get_params"]
    fn _get_params(ptr: u32, size: u32) -> i32;

    #[link_name = "get_input_params"]
    fn _get_input_params(ptr: u32, size: u32) -> i32;
    
    #[link_name = "get_test_missing"]
    fn _get_test_misssing() -> i32;
}

#[no_mangle]
pub extern "C" fn add(x: i32) -> i32 {
    x + unsafe {_get_test_misssing()}
}

