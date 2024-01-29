use lol_alloc::{LeakingPageAllocator};



// SAFETY: This application is single threaded, so using AssumeSingleThreaded is allowed.
#[global_allocator]
static ALLOCATOR: LeakingPageAllocator = LeakingPageAllocator;

pub enum ProgramError {
    // Return error from program
    StateReadError,
    StateWriteError,
    InitReadError,
    ProgramParamsReadError,
}

const SUCCESS: u64 = 0;
const STATE_READ_ERROR: u64 = 1;
const STATE_WRITE_ERROR: u64 = 2;
const INIT_READ_ERROR: u64 = 3;
const INPUT_PARAMS_READ_ERROR: u64 = 4;

const COUNTER_STATE_ID: u32 = 0xaabbccdd;

macro_rules! get_return_on_error {
    ($e:expr) => (match $e {
        Ok(val) => val,
        Err(err) => return err.into(),
    });
}

impl From<ProgramError> for u64 {
    fn from(error: ProgramError) -> Self {
        match error {
            ProgramError::InitReadError => INIT_READ_ERROR,
            ProgramError::StateReadError => STATE_READ_ERROR,
            ProgramError::StateWriteError => STATE_WRITE_ERROR,
            ProgramError::ProgramParamsReadError => INPUT_PARAMS_READ_ERROR,
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

fn save_counter_state(count: u64) -> Result<bool, ProgramError> {
    unsafe {
        let bytes = count.to_le_bytes();
        if  _set_state(COUNTER_STATE_ID, bytes.as_ptr() as u32, bytes.len() as u32) < 0 {
            return Err(ProgramError::StateWriteError);
        }
    }
    Ok(true)
}

fn read_counter_state() -> Result<u64, ProgramError> {
    let v = vec![0u8; 8];
    if unsafe {_get_state(COUNTER_STATE_ID, v.as_ptr() as u32, v.len() as u32)} < v.len() as i32 {
        // state read failed, init counter with data
        if unsafe{_get_params(v.as_ptr() as u32, v.len() as u32)} < v.len() as i32 {
                    return Err(ProgramError::InitReadError);
        }
    }
    Ok(u64::from_le_bytes(v.as_slice().try_into().unwrap()))
}

fn get_input_parameter() -> Result<u64, ProgramError> {
    let v = vec![0u8; 8];
    if unsafe {_get_input_params(v.as_ptr() as u32, v.len() as u32)} < v.len() as i32 {
        return Err(ProgramError::ProgramParamsReadError)
    }       
    // return input parameter as litte endian u64
    Ok(u64::from_le_bytes(v.as_slice().try_into().unwrap()))
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

