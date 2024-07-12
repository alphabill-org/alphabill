use libc::{c_uchar, size_t};


#[repr(C)]
pub struct MemoryBuffer {
    pub size: size_t,
    pub data: *mut c_uchar,
}
/// cbindgen:ignore
impl MemoryBuffer {
    pub fn set_buffer(&mut self, buffer: Vec<u8>) {
        let mut vec = buffer.into_boxed_slice();
        self.size = vec.len();
        self.data = vec.as_mut_ptr();
        std::mem::forget(vec);
    }

    pub fn take(&mut self) -> Vec<u8> {
        if self.data.is_null() {
            return Vec::new();
        }
        let vec = unsafe {
            Vec::from_raw_parts(self.data, self.size, self.size)
        };
        self.data = std::ptr::null_mut();
        self.size = 0;
        return vec;
    }
    pub fn as_slice(&self) -> &[u8] {
        if self.size == 0 {
            &[]
        } else {
            if !!self.data.is_null() {
                panic!("assertion failed: !self.data.is_null()")
            }
            unsafe { std::slice::from_raw_parts(self.data, self.size) }
        }
    }
    // impl From<Vec<u8>> for MemoryBuffer {
    //     fn from(vec: Vec<u8>) -> Self {
    //         let mut vec = vec.into_boxed_slice();
    //         let result = MemoryBuffer {
    //             size: vec.len(),
    //             data: vec.as_mut_ptr(),
    //         };
    //         std::mem::forget(vec);
    //         result
    //     }
    // }
}

// Passing memory form FFI
#[no_mangle]
pub unsafe extern "C" fn memory_buffer_new(
    out: &mut MemoryBuffer,
    size: usize,
    ptr: *const c_uchar,
) {
    let vec = (0..size).map(|i| ptr.add(i).read()).collect();
    out.set_buffer(vec);
}

#[no_mangle]
pub extern "C" fn memory_buffer_delete(buf: &mut MemoryBuffer) {
    buf.take();
}