(module
  (type $t0 (func (result i32)))
  (import "env" "memory" (memory 1))
  (import "env" "p2pkh_v1" (func $p2pkh (type $t0)))
  (func $run (type $t0) (result i32)
    call $p2pkh)
  (export "run" (func $run))
  (global (;0;) i32 (i32.const 1024))
  (export "__heap_base" (global 0)))
