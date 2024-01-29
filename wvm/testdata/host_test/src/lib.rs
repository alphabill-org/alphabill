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
    ParamsReadError,
    StateReadError,
    StateWriteError,
    InputParamsReadError,
}

const SUCCESS: i64 = 0;
const STATE_READ_ERROR: i64 = -1;
const STATE_WRITE_ERROR: i64 = -2;
const PARAMS_READ_ERROR: i64 = -3;
const INPUT_PARAMS_READ_ERROR: i64 = -4;

const STATE_ID: u32 = 0xaabbccdd;
const STATE_SIZE: usize = 8;

static mut G_MEM_BUF: [u8; STATE_SIZE] = [0; STATE_SIZE];

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

impl From<ProgramError> for i64 {
    fn from(error: ProgramError) -> Self {
        match error {
            ProgramError::ParamsReadError => PARAMS_READ_ERROR,
            ProgramError::StateReadError => STATE_READ_ERROR,
            ProgramError::StateWriteError => STATE_WRITE_ERROR,
            ProgramError::InputParamsReadError => INPUT_PARAMS_READ_ERROR,
        }
    }
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
}

/// Note: This doesn't change the ownership of the Array
fn state_to_ptr(s: &[u8; STATE_SIZE]) -> (u32, u32) {
    return (s.as_ptr() as u32, s.len() as u32);
}

fn save_state(count: u64) -> Result<bool, ProgramError> {
    unsafe {
        G_MEM_BUF = count.to_le_bytes();
        let (ptr, len) = state_to_ptr(&G_MEM_BUF);
        if  _set_state(STATE_ID, ptr, len) < 0 {
           return Err(ProgramError::StateWriteError);
        }
    }
    Ok(true)
}

fn read_state() -> Result<u64, ProgramError> {
    unsafe {
        let (ptr, len) = state_to_ptr(&G_MEM_BUF);
        if _get_state(STATE_ID, ptr, len) < 8 {
            return Err(ProgramError::StateReadError);
        }
        Ok(u64::from_le_bytes(G_MEM_BUF))
    }
}

fn get_program_params() -> Result<u64, ProgramError> {
    // state read failed, init counter with data
    unsafe {
        let (ptr, len) = state_to_ptr(&G_MEM_BUF);
        if _get_params(ptr, len) < 8 {
            return Err(ProgramError::ParamsReadError);
        }
        Ok(u64::from_le_bytes(G_MEM_BUF))
    }    
}

fn get_input_parameter() -> Result<u64, ProgramError> {
    unsafe {
        let (ptr, len) = state_to_ptr(&G_MEM_BUF);
        if _get_input_params(ptr, len) < 8 {
            return Err(ProgramError::InputParamsReadError)
        }
        // return input params
        Ok(u64::from_le_bytes(G_MEM_BUF))
    }
}

#[no_mangle]
pub extern "C" fn set_state_input() -> i64 {
    // get program input parameter
    let input = get_return_on_error!(get_input_parameter());
    get_return_on_error!(save_state(input));
    SUCCESS
}

#[no_mangle]
pub extern "C" fn set_state_params() -> i64 {
    // get program input parameter
    let params = get_return_on_error!(get_program_params());
    get_return_on_error!(save_state(params));
    SUCCESS
}

#[no_mangle]
pub extern "C" fn get_state() -> i64 {
    // get program input parameter
    let state = get_return_on_error!(read_state());
    state as i64   
}

