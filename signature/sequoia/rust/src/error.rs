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
    message: *const c_char,
}

#[no_mangle]
pub unsafe extern "C" fn sequoia_error_free(err_ptr: *mut SequoiaError) {
    drop(Box::from_raw(err_ptr))
}

pub unsafe fn set_error(err_ptr: *mut *mut SequoiaError, kind: SequoiaErrorKind, message: &str) {
    if !err_ptr.is_null() {
        *err_ptr = Box::into_raw(Box::new(SequoiaError {
            kind,
            message: CString::new(message).unwrap().into_raw(),
        }));
    }
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
