(module
  (type $t0 (func (result i32)))
  (import "ab" "p2pkh_v1" (func $p2pkh (type $t0)))
  (func $run (type $t0) (result i32)
    call $p2pkh)
  (export "run" (func $run)))