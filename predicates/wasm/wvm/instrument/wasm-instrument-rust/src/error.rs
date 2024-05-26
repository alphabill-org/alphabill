use std::fmt::Display;
use std::cell::RefCell;

thread_local! {
    static ERROR_STR: RefCell<Option<String>> = RefCell::new(None);
}

pub fn set_error<E: Display>(new_err: E) {
    ERROR_STR.with(|err| {*err.borrow_mut() = Some(new_err.to_string())});
}

/// Retrieve the most recent error, clearing it in the process.
pub(crate) fn get_error() -> Option<String> {
   ERROR_STR.with(|err| err.borrow_mut().take())
}