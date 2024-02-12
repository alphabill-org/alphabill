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


// not public and also not marked #[no_mangle]
// since it is not marked public the compiler will optimize and remove this method
fn add(x: i32) -> i32 {
    x + 1
}

