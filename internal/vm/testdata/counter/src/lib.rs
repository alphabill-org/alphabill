#![no_std]

/* 
use {
    core::slice::from_raw_parts,
    core::mem::size_of,
    core::result::Result as ResultGeneric
};
type ProgramResult = ResultGeneric<(), ProgramError>;
*/
 
pub enum ProgramError {
    // Return error from program
    InvalidArgument,
    InvalidInstruction,
    InitReadError,
    StateReadError,
    StateWriteError,
    ProgramParamsReadError,
}


const SUCCESS: u64 = 0;
const INVALID_ARGUMENT: u64 = 1;
const INVALID_INSTRUCTION: u64 = 2;
const INIT_READ_ERROR: u64 = 3;
const STATE_READ_ERROR: u64 = 4;
const STATE_WRITE_ERROR: u64 = 5;
const INPUT_PARAMS_READ_ERROR: u64 = 6;

const COUNTER_STATE_ID: u32 = 0xaabbccdd;
const STATE_SIZE: usize = 8;

static mut PROGRAM_STATE: [u8; STATE_SIZE] = [0; STATE_SIZE];

macro_rules! get_return_on_error {
    ($e:expr) => (match $e {
        Ok(val) => val,
        Err(err) => return err.into(),
    });
}

/// This function is called on panic.
// Need to provide a tiny `panic` implementation for `#![no_std]`.
// This translates into an `unreachable` instruction that will
// raise a `trap` the WebAssembly execution if we panic at runtime.
#[cfg(not(test))]
#[panic_handler]
fn panic(_panic: &core::panic::PanicInfo<'_>) -> ! {
    core::arch::wasm32::unreachable()
}

impl From<ProgramError> for u64 {
    fn from(error: ProgramError) -> Self {
        match error {
            ProgramError::InvalidArgument => INVALID_ARGUMENT,
            ProgramError::InvalidInstruction => INVALID_INSTRUCTION,
            ProgramError::InitReadError => INIT_READ_ERROR,
            ProgramError::StateReadError => STATE_READ_ERROR,
            ProgramError::StateWriteError => STATE_WRITE_ERROR,
            ProgramError::ProgramParamsReadError => INPUT_PARAMS_READ_ERROR,
        }
    }
}

#[link(wasm_import_module = "ab_v0")]
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
}

/// Note: This doesn't change the ownership of the Array
fn state_to_ptr(s: &[u8; STATE_SIZE]) -> (u32, u32) {
    return (s.as_ptr() as u32, s.len() as u32);
}

fn save_counter_state(count: u64) -> Result<bool, ProgramError> {
    unsafe {
        PROGRAM_STATE = count.to_le_bytes();
    }
    let (ptr, len) = state_to_ptr(unsafe { &PROGRAM_STATE });
    unsafe {
        if  _set_state(COUNTER_STATE_ID, ptr, len) < 0 {
           return Err(ProgramError::StateWriteError);
        }
    }
    Ok(true)
}

fn read_counter_state() -> Result<u64, ProgramError> {
    let (ptr, len) = state_to_ptr(unsafe { &PROGRAM_STATE });
    unsafe {
        if _get_state(COUNTER_STATE_ID, ptr, len) < 0 {
            // state read failed, init counter with data
            if _get_params(ptr, len) < 0 {
                return Err(ProgramError::InitReadError);
            }
        }
    }
    let counter =  u64::from_le_bytes(unsafe { PROGRAM_STATE });
    return Ok(counter)
}

fn get_input_parameter() -> Result<u64, ProgramError> {
    // NB! reusing global static buffer
    let (ptr, len) = state_to_ptr(unsafe { &PROGRAM_STATE });
    unsafe {
        if _get_input_params(ptr, len) < 8 {
            return Err(ProgramError::ProgramParamsReadError)
        }
    }
    // return count value
    Ok(u64::from_le_bytes(unsafe { PROGRAM_STATE }))
}

#[no_mangle]
pub extern "C" fn count() -> u64 {
    // get program input parameter
    let inc = get_return_on_error!(get_input_parameter());
    let mut counter = get_return_on_error!(read_counter_state());
    // increment counter
    counter += inc;
    get_return_on_error!(save_counter_state(counter));
    SUCCESS      
}

/* 
pub unsafe fn deserialize<'a>(input: *mut u8, len: u32) -> (u16, &'a [u8]) {
    let mut offset: usize = 0;
    let instruction = *(input.add(offset) as *const u16) as u16;
    offset += size_of::<u16>();
    let input_data = { from_raw_parts(input.add(offset), len as usize-size_of::<u16>()) };
    (instruction, input_data)
}
*/
/* 
#[cfg(test)]
#[warn(unused_imports)]
mod tests {
    use super::*;
    pub mod overrides {
        extern "C" fn _get_state(id: u32, ptr: u32, size: u32) -> i32 {
           return 0
        }
        extern "C" fn _set_state(id: u32, ptr: u32, size: u32) -> i32 {
            return 0
        }
        extern "C" fn _get_params(ptr: u32, size: u32) -> i32{
            return 0;
        }
        
        extern "C" fn _get_input_params(ptr: u32, size: u32) -> i32{
            return 0;
        }
        }
        
    #[test]
    fn it_works() {
        core::assert_eq!(count(), 0);
    }
}
*/
