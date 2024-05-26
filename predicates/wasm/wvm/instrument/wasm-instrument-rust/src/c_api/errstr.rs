use std::ffi::CString;
use libc::c_char;

#[no_mangle]
pub unsafe extern "C" fn errstr() -> *mut c_char { 
    let err_str = match crate::error::get_error(){
        Some(err) => err,
        None => return std::ptr::null_mut(),
    };
    let c_err = CString::new(err_str).unwrap();
     c_err.into_raw()
}

#[no_mangle]
pub unsafe extern "C" fn errstr_free(err_str: *mut c_char) { 
    unsafe {
        if err_str.is_null() {
            return;
        }
        let _ = CString::from_raw(err_str);
    };
}