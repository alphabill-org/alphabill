use std::fmt;
use wasm_instrument::{
    gas_metering,
    gas_metering::{host_function, mutable_global, ConstantCostRules},
    inject_stack_limiter,
    parity_wasm::{deserialize_buffer, elements::Module, serialize},
};

#[derive(Debug, Clone)]
pub enum Error {
    ErrEmptyWasm,
    /// Other allocated error.
    SerializationError(String),
    /// Stack metering error
    StackMeterErr(&'static str),
    /// Gas metering failed
    GasMeteringErr,
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match *self {
            Error::ErrEmptyWasm => write!(f, "empty input as wasm module"),
            Error::SerializationError(ref msg) => write!(f, "{}", msg),
            Error::StackMeterErr(msg) => write!(f, "{}", msg),
            Error::GasMeteringErr => write!(f, "gas metering injection failed"),
        }
    }
}

#[cfg(feature = "std")]
impl ::std::error::Error for Error {
    fn description(&self) -> &str {
        match *self {
            Error::ErrEmptyWasm => "empty input as wasm module",
            Error::IOError(msg) => msg,
        }
    }
}

/// Error wrapper for Serialize errors
impl From<wasm_instrument::parity_wasm::SerializationError> for Error {
    fn from(err: wasm_instrument::parity_wasm::SerializationError) -> Self {
        Error::SerializationError(format!("Serialize error: {:?}", err))
    }
}

/// Error wrapper for Gas Injection errors
impl From<Module> for Error {
    fn from(err: Module) -> Self {
        Error::SerializationError(format!("Serialize error: {:?}", err))
    }
}

/// Inject gas metering
pub fn inject_gas_metering_using_global_cnt(
    stack_limit: u32,
    wasm: &[u8],
) -> Result<Vec<u8>, Error> {
    if wasm.len() == 0 {
        return Err(Error::ErrEmptyWasm.into());
    }

    let mut module: Module = match deserialize_buffer(&wasm) {
        Ok(deserialized) => deserialized,
        Err(err_msg) => return Err(Error::SerializationError(err_msg.to_string())),
    };
    // instrument for gas
    module = match gas_metering::inject(
        module,
        mutable_global::Injector::new("gas_count"),
        &ConstantCostRules::default(),
    ) {
        Ok(res_mod) => res_mod,
        Err(_) => return Err(Error::GasMeteringErr),
    };
    // instrument for stack size
    if stack_limit != 0 {
        module = match inject_stack_limiter(module, stack_limit) {
            Ok(res_mod) => res_mod,
            Err(err_msg) => return Err(Error::StackMeterErr(err_msg).into()),
        }
    }
    let instrumented_wasm = serialize(module)?;
    Ok(instrumented_wasm)
}

/// Inject gas metering
#[allow(dead_code)]
pub fn inject_gas_metering_using_hostfn(stack_limit: u32, wasm: &[u8]) -> Result<Vec<u8>, Error> {
    if wasm.len() == 0 {
        return Err(Error::ErrEmptyWasm.into());
    }
    let mut module: Module = deserialize_buffer(&wasm)?;
    // instrument for gas
    module = gas_metering::inject(
        module,
        host_function::Injector::new("env", "gas"),
        &ConstantCostRules::default(),
    )?;
    // instrument for stack size
    if stack_limit != 0 {
        module = match inject_stack_limiter(module, stack_limit) {
            Ok(res_mod) => res_mod,
            Err(err_msg) => return Err(Error::StackMeterErr(err_msg).into()),
        }
    }
    let instrumented_wasm = serialize(module)?;
    Ok(instrumented_wasm)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_empty_input_error() {
        let wasm: &[u8] = &[];
        let result = inject_gas_metering_using_global_cnt(0, wasm);
        assert!(result.is_err());
        let result = inject_gas_metering_using_hostfn(0, wasm);
        assert!(result.is_err());
    }
}
