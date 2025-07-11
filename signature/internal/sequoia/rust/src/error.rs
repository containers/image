// SPDX-License-Identifier: LGPL-2.0-or-later

#![allow(clippy::missing_safety_doc)]
use libc::c_char;
use std::ffi::CString;
use std::io;

#[repr(C)]
pub enum SequoiaErrorKind {
    Unknown,
    InvalidArgument,
    IoError,
}

#[repr(C)]
pub struct SequoiaError {
    kind: SequoiaErrorKind,
    message: *mut c_char,
}

impl Drop for SequoiaError {
    fn drop(&mut self) {
        unsafe {
            let _ = CString::from_raw(self.message);
        }
    }
}

#[no_mangle]
pub unsafe extern "C" fn sequoia_error_free(err_ptr: *mut SequoiaError) {
    drop(Box::from_raw(err_ptr))
}

pub unsafe fn set_error_from(err_ptr: *mut *mut SequoiaError, err: anyhow::Error) {
    if !err_ptr.is_null() {
        let kind = if err.is::<io::Error>() {
            SequoiaErrorKind::IoError
        } else {
            SequoiaErrorKind::Unknown
        };

        *err_ptr = Box::into_raw(Box::new(SequoiaError {
            kind,
            message: CString::from_vec_unchecked(err.to_string().into()).into_raw(),
        }));
    }
}
