
use core::mem;
use core::marker::PhantomData;
use crate::env::malloc;
use crate::env::free;

/// Values supported by Substrate on the boundary between host/Wasm.
//  codec::Encode, codec::Decode
#[derive(PartialEq, Debug, Clone, Copy)]
pub enum Value {
	/// A 32-bit integer.
	I32(i32),
	/// A 64-bit integer.
	I64(i64),
	/// A 32-bit floating-point number stored as raw bit pattern.
	///
	/// You can materialize this value using `f32::from_bits`.
	F32(u32),
	/// A 64-bit floating-point number stored as raw bit pattern.
	///
	/// You can materialize this value using `f64::from_bits`.
	F64(u64),
}

impl Value {
	/// Returns the type of this value.
	pub fn value_type(&self) -> ValueType {
		match self {
			Value::I32(_) => ValueType::I32,
			Value::I64(_) => ValueType::I64,
			Value::F32(_) => ValueType::F32,
			Value::F64(_) => ValueType::F64,
		}
	}

	/// Return `Self` as `i32`.
	pub fn as_i32(&self) -> Option<i32> {
		match self {
			Self::I32(val) => Some(*val),
			_ => None,
		}
	}
}

/// Provides `Sealed` trait to prevent implementing trait `PointerType` and `WasmTy` outside of this
/// crate.
mod private {
	pub trait Sealed {}

	impl Sealed for u8 {}
	impl Sealed for u16 {}
	impl Sealed for u32 {}
	impl Sealed for u64 {}

	impl Sealed for i32 {}
	impl Sealed for i64 {}
}

/// Value types supported by Substrate on the boundary between host/Wasm.
#[derive(Copy, Clone, PartialEq, Debug, Eq)]
pub enum ValueType {
	/// An `i32` value type.
	I32,
	/// An `i64` value type.
	I64,
	/// An `f32` value type.
	F32,
	/// An `f64` value type.
	F64,
}

/// Something that can be wrapped in a wasm `Pointer`.
///
/// This trait is sealed.
pub trait PointerType: Sized + private::Sealed {
	/// The size of the type in wasm.
	const SIZE: u32 = mem::size_of::<Self>() as u32;
}

impl PointerType for u8 {}
impl PointerType for u16 {}
impl PointerType for u32 {}
impl PointerType for u64 {}

/// Type to represent a pointer in wasm at the host.
#[derive(Debug, PartialEq, Eq, Clone, Copy)]
pub struct Pointer<T: PointerType> {
	ptr: u32,
	_marker: PhantomData<T>,
}

impl<T: PointerType> Pointer<T> {
	/// Create a new instance of `Self`.
	pub fn new(ptr: u32) -> Self {
		Self { ptr, _marker: Default::default() }
	}

	/// Calculate the offset from this pointer.
	///
	/// `offset` is in units of `T`. So, `3` means `3 * mem::size_of::<T>()` as offset to the
	/// pointer.
	///
	/// Returns an `Option` to respect that the pointer could probably overflow.
	pub fn offset(self, offset: u32) -> Option<Self> {
		offset
			.checked_mul(T::SIZE)
			.and_then(|o| self.ptr.checked_add(o))
			.map(|ptr| Self { ptr, _marker: Default::default() })
	}

	/// Create a null pointer.
	pub fn null() -> Self {
		Self::new(0)
	}

	/// Cast this pointer of type `T` to a pointer of type `R`.
	pub fn cast<R: PointerType>(self) -> Pointer<R> {
		Pointer::new(self.ptr)
	}
}

impl<T: PointerType> From<u32> for Pointer<T> {
	fn from(ptr: u32) -> Self {
		Pointer::new(ptr)
	}
}

impl<T: PointerType> From<Pointer<T>> for u32 {
	fn from(ptr: Pointer<T>) -> Self {
		ptr.ptr
	}
}

impl<T: PointerType> From<Pointer<T>> for u64 {
	fn from(ptr: Pointer<T>) -> Self {
		u64::from(ptr.ptr)
	}
}

impl<T: PointerType> From<Pointer<T>> for usize {
	fn from(ptr: Pointer<T>) -> Self {
		ptr.ptr as _
	}
}

impl<T: PointerType> IntoValue for Pointer<T> {
	const VALUE_TYPE: ValueType = ValueType::I32;
	fn into_value(self) -> Value {
		Value::I32(self.ptr as _)
	}
}

impl<T: PointerType> TryFromValue for Pointer<T> {
	fn try_from_value(val: Value) -> Option<Self> {
		match val {
			Value::I32(val) => Some(Self::new(val as _)),
			_ => None,
		}
	}
}

/// The word size used in wasm. Normally known as `usize` in Rust.
pub type WordSize = u32;

/// Something that can be converted into a wasm compatible `Value`.
pub trait IntoValue {
	/// The type of the value in wasm.
	const VALUE_TYPE: ValueType;

	/// Convert `self` into a wasm `Value`.
	fn into_value(self) -> Value;
}

/// Something that can may be created from a wasm `Value`.
pub trait TryFromValue: Sized {
	/// Try to convert the given `Value` into `Self`.
	fn try_from_value(val: Value) -> Option<Self>;
}

/// Allocator used by Substrate when executing the Wasm runtime.
struct WasmAllocator;

#[global_allocator]
static ALLOCATOR: WasmAllocator = WasmAllocator;

mod allocator_impl {
	use super::*;
	use core::alloc::{GlobalAlloc, Layout};

	unsafe impl GlobalAlloc for WasmAllocator {
		unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
			malloc(layout.size() as u32) as *mut u8
		}

		unsafe fn dealloc(&self, ptr: *mut u8, _: Layout) {
			free(ptr as u32)
		}
	}
}
