use super::types::MemoryBuffer;
extern crate libc;

#[no_mangle]
pub extern "C" fn instrument_wasm(
    wasm: &MemoryBuffer,
    stack_limit: u32,
    out: &mut MemoryBuffer,
) -> i32 {
    if wasm.data.is_null() {
        // No data there, already freed probably.
        return 1;
    }
    let wasm = wasm.as_slice();

    match crate::instrument::inject_gas_metering_using_global_cnt(stack_limit, wasm) {
        Ok(res_mod) => out.set_buffer(res_mod),
        Err(err) => {
            crate::error::set_error(err);
            return 1;
        }
    };
    0
}


